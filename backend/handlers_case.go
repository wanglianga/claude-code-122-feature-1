package main

import (
	"fmt"
	"net/http"
	"strings"
)

// handleCaseList 协同工单列表（全角色可见，便于同桶/同批协同处理）。
func (s *Server) handleCaseList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if v := q.Get("status"); v != "" {
		add("c.status = $%d", v)
	}
	if v := q.Get("type"); v != "" {
		add("c.case_type = $%d", v)
	}
	rows, err := s.db.Query(`
		SELECT c.id, c.case_no, c.case_type, c.status, c.title, c.detail,
			COALESCE(k.keg_no,''), COALESCE(b.batch_no,''), COALESCE(br.name,''),
			to_char(c.created_at,'YYYY-MM-DD HH24:MI'),
			(SELECT count(*) FROM case_notes n WHERE n.case_id=c.id)
		FROM cases c
		LEFT JOIN kegs k ON k.id=c.keg_id
		LEFT JOIN batches b ON b.id=c.batch_id
		LEFT JOIN bars br ON br.id=c.bar_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY c.id DESC`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, notes int
		var no, ctype, status, title, detail, kegNo, batchNo, barName, ts string
		if err := rows.Scan(&id, &no, &ctype, &status, &title, &detail, &kegNo, &batchNo, &barName, &ts, &notes); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "case_no": no, "case_type": ctype, "status": status, "title": title, "detail": detail,
			"keg_no": kegNo, "batch_no": batchNo, "bar_name": barName, "created_at": ts, "note_count": notes,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCaseDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效工单 ID")
		return
	}
	var no, ctype, status, title, detail, resolution, kegNo, batchNo, barName, ts string
	var resolvedAt *string
	var kegID, batchID, barID *int
	err = s.db.QueryRow(`
		SELECT c.case_no, c.case_type, c.status, c.title, c.detail, c.resolution,
			c.keg_id, c.batch_id, c.bar_id,
			COALESCE(k.keg_no,''), COALESCE(b.batch_no,''), COALESCE(br.name,''),
			to_char(c.created_at,'YYYY-MM-DD HH24:MI'), to_char(c.resolved_at,'YYYY-MM-DD HH24:MI')
		FROM cases c
		LEFT JOIN kegs k ON k.id=c.keg_id
		LEFT JOIN batches b ON b.id=c.batch_id
		LEFT JOIN bars br ON br.id=c.bar_id
		WHERE c.id=$1`, id).
		Scan(&no, &ctype, &status, &title, &detail, &resolution, &kegID, &batchID, &barID,
			&kegNo, &batchNo, &barName, &ts, &resolvedAt)
	if err != nil {
		writeErr(w, 404, "工单不存在")
		return
	}
	notes := []map[string]any{}
	rows, err := s.db.Query(`SELECT user_name, role, note, to_char(created_at,'YYYY-MM-DD HH24:MI')
		FROM case_notes WHERE case_id=$1 ORDER BY id`, id)
	if err == nil {
		for rows.Next() {
			var name, role, note, nts string
			_ = rows.Scan(&name, &role, &note, &nts)
			notes = append(notes, map[string]any{"user_name": name, "role": role, "note": note, "time": nts})
		}
		rows.Close()
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "case_no": no, "case_type": ctype, "status": status, "title": title, "detail": detail,
		"resolution": resolution, "keg_id": kegID, "batch_id": batchID, "bar_id": barID,
		"keg_no": kegNo, "batch_no": batchNo, "bar_name": barName,
		"created_at": ts, "resolved_at": resolvedAt, "notes": notes,
	})
}

func (s *Server) handleCaseNote(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	id, _ := pathID(r, "id")
	var req struct {
		Note string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil || req.Note == "" {
		writeErr(w, 400, "备注不能为空")
		return
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM cases WHERE id=$1`, id).Scan(&status); err != nil {
		writeErr(w, 404, "工单不存在")
		return
	}
	if _, err := s.db.Exec(`INSERT INTO case_notes (case_id, user_id, role, user_name, note) VALUES ($1,$2,$3,$4,$5)`,
		id, u.ID, u.Role, u.Name, req.Note); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if status == "open" {
		_, _ = s.db.Exec(`UPDATE cases SET status='processing' WHERE id=$1`, id)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// handleCaseResolve 结案：可附带押金扣减（财务/仓库/品控/管理员）。
func (s *Server) handleCaseResolve(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse", "qc", "finance") {
		writeErr(w, 403, "仅仓库/品控/财务可结案")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		Resolution      string  `json:"resolution"`
		DeductionAmount float64 `json:"deduction_amount"`
	}
	if err := decodeBody(r, &req); err != nil || req.Resolution == "" {
		writeErr(w, 400, "请填写处理结论")
		return
	}
	var status, caseNo string
	var kegID, barID *int
	err := s.db.QueryRow(`SELECT status, case_no, keg_id, bar_id FROM cases WHERE id=$1`, id).
		Scan(&status, &caseNo, &kegID, &barID)
	if err != nil {
		writeErr(w, 404, "工单不存在")
		return
	}
	if status == "resolved" {
		writeErr(w, 400, "工单已结案")
		return
	}
	if _, err := s.db.Exec(`UPDATE cases SET status='resolved', resolution=$1, resolved_at=now() WHERE id=$2`,
		req.Resolution, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.db.Exec(`INSERT INTO case_notes (case_id, user_id, role, user_name, note) VALUES ($1,$2,$3,$4,$5)`,
		id, u.ID, u.Role, u.Name, "【结案】"+req.Resolution)

	// 附带押金扣减
	if req.DeductionAmount > 0 && kegID != nil && barID != nil {
		var kegNo string
		_ = s.db.QueryRow(`SELECT keg_no FROM kegs WHERE id=$1`, *kegID).Scan(&kegNo)
		_, _ = s.db.Exec(`INSERT INTO deposit_ledger (keg_id, bar_id, entry_type, amount, reason, period, status, created_by)
			VALUES ($1,$2,'deduction',$3,$4,$5,'pending',$6)`,
			*kegID, *barID, req.DeductionAmount,
			fmt.Sprintf("工单 %s 结案扣款：%s", caseNo, req.Resolution), periodNow(), u.ID)
		s.notify(nil, "finance", nil, nil, "deposit", "押金扣款待确认",
			fmt.Sprintf("工单 %s 结案，建议扣减酒桶 %s 押金 ¥%.2f。", caseNo, kegNo, req.DeductionAmount))
		s.notify(nil, "", barID, nil, "deposit", "押金扣款通知",
			fmt.Sprintf("因工单 %s（%s），贵店酒桶 %s 押金将扣减 ¥%.2f。", caseNo, req.Resolution, kegNo, req.DeductionAmount))
	}
	writeJSON(w, 200, map[string]string{"status": "resolved"})
}
