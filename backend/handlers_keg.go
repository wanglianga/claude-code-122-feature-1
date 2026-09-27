package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// handleKegList 按状态/批次/酒吧过滤酒桶；酒吧店长只看本店，经销商只看下辖酒吧。
func (s *Server) handleKegList(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if v := q.Get("status"); v != "" {
		add("k.status = $%d", v)
	}
	if v := q.Get("batch_id"); v != "" {
		add("k.batch_id = $%d", v)
	}
	if v := q.Get("bar_id"); v != "" {
		add("k.current_bar_id = $%d", v)
	}
	if v := q.Get("q"); v != "" {
		add("k.keg_no ILIKE '%%' || $%d || '%%'", v)
	}
	if u.Role == "bar_manager" && u.BarID != nil {
		add("k.current_bar_id = $%d", *u.BarID)
	}
	if u.Role == "dealer" && u.DealerID != nil {
		add("br.dealer_id = $%d", *u.DealerID)
	}
	rows, err := s.db.Query(`
		SELECT k.id, k.keg_no, k.capacity_liters, k.batch_id, COALESCE(b.batch_no,''), COALESCE(b.style,''),
			k.deposit_amount, k.status, COALESCE(br.name,''), k.refill_eligible,
			COALESCE(to_char(k.delivered_at,'YYYY-MM-DD'),'')
		FROM kegs k
		LEFT JOIN batches b ON b.id=k.batch_id
		LEFT JOIN bars br ON br.id=k.current_bar_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY k.keg_no`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, cap, batchID int
		var kegNo, batchNo, style, status, barName, delivered string
		var dep float64
		var eligible bool
		if err := rows.Scan(&id, &kegNo, &cap, &batchID, &batchNo, &style, &dep, &status, &barName, &eligible, &delivered); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "keg_no": kegNo, "capacity_liters": cap, "batch_id": batchID,
			"batch_no": batchNo, "style": style, "deposit_amount": dep, "status": status,
			"bar_name": barName, "refill_eligible": eligible, "delivered_at": delivered,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleKegCreate(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse") {
		writeErr(w, 403, "仅酒厂仓库可登记酒桶")
		return
	}
	var req struct {
		KegNo    string  `json:"keg_no"`
		Capacity int     `json:"capacity_liters"`
		BatchID  int     `json:"batch_id"`
		Deposit  float64 `json:"deposit_amount"`
	}
	if err := decodeBody(r, &req); err != nil || req.KegNo == "" || req.Capacity <= 0 || req.BatchID == 0 {
		writeErr(w, 400, "桶号/容量/批次为必填")
		return
	}
	if req.Deposit <= 0 {
		req.Deposit = 200
	}
	var batchNo, batchStatus string
	var hold bool
	if err := s.db.QueryRow(`SELECT batch_no, status, delivery_hold FROM batches WHERE id=$1`, req.BatchID).Scan(&batchNo, &batchStatus, &hold); err != nil {
		writeErr(w, 400, "批次不存在")
		return
	}
	if batchStatus != "active" {
		writeErr(w, 400, "该批次已停售/召回，不能再灌装")
		return
	}
	if hold {
		writeErr(w, 400, "该批次口味排查中，已暂停灌装与配送")
		return
	}
	var id int
	err := s.db.QueryRow(`INSERT INTO kegs (keg_no, capacity_liters, batch_id, deposit_amount) VALUES ($1,$2,$3,$4) RETURNING id`,
		req.KegNo, req.Capacity, req.BatchID, req.Deposit).Scan(&id)
	if err != nil {
		writeErr(w, 400, "登记失败（桶号可能重复）："+err.Error())
		return
	}
	s.logEvent(id, "灌装入库", u, fmt.Sprintf("批次 %s 灌装，入酒厂仓库", batchNo))
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) handleKegDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效酒桶 ID")
		return
	}
	var (
		kegNo, status, batchNo, style, batchStatus, barName string
		cap, batchID                                        int
		dep                                                 float64
		eligible                                            bool
		delivered, opened, emptied, returned                *string
	)
	err = s.db.QueryRow(`
		SELECT k.keg_no, k.capacity_liters, k.batch_id, COALESCE(b.batch_no,''), COALESCE(b.style,''),
			COALESCE(b.status,''), k.deposit_amount, k.status, COALESCE(br.name,''), k.refill_eligible,
			to_char(k.delivered_at,'YYYY-MM-DD HH24:MI'), to_char(k.opened_at,'YYYY-MM-DD HH24:MI'),
			to_char(k.emptied_at,'YYYY-MM-DD HH24:MI'), to_char(k.returned_at,'YYYY-MM-DD HH24:MI')
		FROM kegs k
		LEFT JOIN batches b ON b.id=k.batch_id
		LEFT JOIN bars br ON br.id=k.current_bar_id
		WHERE k.id=$1`, id).
		Scan(&kegNo, &cap, &batchID, &batchNo, &style, &batchStatus, &dep, &status, &barName, &eligible,
			&delivered, &opened, &emptied, &returned)
	if err != nil {
		writeErr(w, 404, "酒桶不存在")
		return
	}
	out := map[string]any{
		"id": id, "keg_no": kegNo, "capacity_liters": cap, "batch_id": batchID, "batch_no": batchNo,
		"style": style, "batch_status": batchStatus, "deposit_amount": dep, "status": status,
		"bar_name": barName, "refill_eligible": eligible,
		"delivered_at": delivered, "opened_at": opened, "emptied_at": emptied, "returned_at": returned,
	}

	events := []map[string]any{}
	rows, err := s.db.Query(`SELECT event, actor_name, detail, to_char(created_at,'YYYY-MM-DD HH24:MI')
		FROM keg_events WHERE keg_id=$1 ORDER BY id`, id)
	if err == nil {
		for rows.Next() {
			var ev, actor, detail, ts string
			_ = rows.Scan(&ev, &actor, &detail, &ts)
			events = append(events, map[string]any{"event": ev, "actor": actor, "detail": detail, "time": ts})
		}
		rows.Close()
	}
	out["events"] = events

	feedbacks := []map[string]any{}
	rows, err = s.db.Query(`SELECT f.pour_speed, f.pressure_issue, f.taste_issue, f.complaint, f.equipment, f.photos, f.detail,
		COALESCE(br.name,''), to_char(f.created_at,'YYYY-MM-DD HH24:MI')
		FROM keg_feedback f LEFT JOIN bars br ON br.id=f.bar_id WHERE f.keg_id=$1 ORDER BY f.id`, id)
	if err == nil {
		for rows.Next() {
			var pour, equipment, photosJSON, detail, bar, ts string
			var p, t, c bool
			_ = rows.Scan(&pour, &p, &t, &c, &equipment, &photosJSON, &detail, &bar, &ts)
			photos := []string{}
			if photosJSON != "" {
				_ = json.Unmarshal([]byte(photosJSON), &photos)
			}
			feedbacks = append(feedbacks, map[string]any{
				"pour_speed": pour, "pressure_issue": p, "taste_issue": t, "complaint": c,
				"equipment": equipment, "photos": photos, "detail": detail, "bar_name": bar, "time": ts,
			})
		}
		rows.Close()
	}
	out["feedbacks"] = feedbacks

	washes := []map[string]any{}
	rows, err = s.db.Query(`SELECT w.result, w.note, COALESCE(u.name,''), to_char(w.created_at,'YYYY-MM-DD HH24:MI')
		FROM wash_records w LEFT JOIN users u ON u.id=w.washer_id WHERE w.keg_id=$1 ORDER BY w.id`, id)
	if err == nil {
		for rows.Next() {
			var result, note, washer, ts string
			_ = rows.Scan(&result, &note, &washer, &ts)
			washes = append(washes, map[string]any{"result": result, "note": note, "washer": washer, "time": ts})
		}
		rows.Close()
	}
	out["wash_records"] = washes

	deposits := []map[string]any{}
	rows, err = s.db.Query(`SELECT entry_type, amount, reason, period, status, to_char(created_at,'YYYY-MM-DD HH24:MI')
		FROM deposit_ledger WHERE keg_id=$1 ORDER BY id`, id)
	if err == nil {
		for rows.Next() {
			var etype, reason, period, st, ts string
			var amount float64
			_ = rows.Scan(&etype, &amount, &reason, &period, &st, &ts)
			deposits = append(deposits, map[string]any{
				"entry_type": etype, "amount": amount, "reason": reason, "period": period, "status": st, "time": ts,
			})
		}
		rows.Close()
	}
	out["deposits"] = deposits

	cases := []map[string]any{}
	rows, err = s.db.Query(`SELECT id, case_no, case_type, status, title FROM cases WHERE keg_id=$1 ORDER BY id DESC`, id)
	if err == nil {
		for rows.Next() {
			var cid int
			var cno, ctype, cstatus, title string
			_ = rows.Scan(&cid, &cno, &ctype, &cstatus, &title)
			cases = append(cases, map[string]any{"id": cid, "case_no": cno, "case_type": ctype, "status": cstatus, "title": title})
		}
		rows.Close()
	}
	out["cases"] = cases
	writeJSON(w, 200, out)
}

// kegAtBar 校验酒桶属于当前店长所在酒吧。
func (s *Server) kegAtBar(kegID int, u *User) (string, string, error) {
	var status, batchStatus string
	var barID *int
	err := s.db.QueryRow(`SELECT k.status, k.current_bar_id, COALESCE(b.status,'active')
		FROM kegs k LEFT JOIN batches b ON b.id=k.batch_id WHERE k.id=$1`, kegID).
		Scan(&status, &barID, &batchStatus)
	if err != nil {
		return "", "", fmt.Errorf("酒桶不存在")
	}
	if u.Role == "bar_manager" {
		if u.BarID == nil || barID == nil || *u.BarID != *barID {
			return "", "", fmt.Errorf("该酒桶不在贵店")
		}
	}
	return status, batchStatus, nil
}

func (s *Server) handleKegOpen(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "bar_manager") {
		writeErr(w, 403, "仅酒吧店长可开桶")
		return
	}
	id, _ := pathID(r, "id")
	status, batchStatus, err := s.kegAtBar(id, u)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if status != "at_bar" {
		writeErr(w, 400, "仅在店未开桶的酒桶可以开桶")
		return
	}
	if batchStatus != "active" {
		writeErr(w, 400, "该批次已停售/召回，禁止开桶")
		return
	}
	if _, err := s.db.Exec(`UPDATE kegs SET status='opened', opened_at=now() WHERE id=$1`, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logEvent(id, "开桶", u, "开桶上枪")
	writeJSON(w, 200, map[string]string{"status": "opened"})
}

func (s *Server) handleKegFeedback(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "bar_manager") {
		writeErr(w, 403, "仅酒吧店长可提交开桶反馈")
		return
	}
	id, _ := pathID(r, "id")
	status, _, err := s.kegAtBar(id, u)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if status != "opened" && status != "at_bar" {
		writeErr(w, 400, "仅在店酒桶可提交反馈")
		return
	}
	var req struct {
		PourSpeed     string   `json:"pour_speed"`
		PressureIssue bool     `json:"pressure_issue"`
		TasteIssue    bool     `json:"taste_issue"`
		Complaint     bool     `json:"complaint"`
		Equipment     string   `json:"equipment"`
		Photos        []string `json:"photos"`
		Detail        string   `json:"detail"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if len(req.Photos) > 3 {
		writeErr(w, 400, "客诉照片最多 3 张")
		return
	}
	for _, p := range req.Photos {
		if len(p) > 800*1024 {
			writeErr(w, 400, "单张照片不能超过约 600KB，请压缩后上传")
			return
		}
	}
	photosJSON := ""
	if len(req.Photos) > 0 {
		if b, err := json.Marshal(req.Photos); err == nil {
			photosJSON = string(b)
		}
	}
	var batchID int
	var kegNo, batchNo string
	_ = s.db.QueryRow(`SELECT k.batch_id, k.keg_no, COALESCE(b.batch_no,'') FROM kegs k
		LEFT JOIN batches b ON b.id=k.batch_id WHERE k.id=$1`, id).Scan(&batchID, &kegNo, &batchNo)
	if _, err := s.db.Exec(`INSERT INTO keg_feedback (keg_id, batch_id, bar_id, pour_speed, pressure_issue, taste_issue, complaint, equipment, photos, detail, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		id, batchID, u.BarID, req.PourSpeed, req.PressureIssue, req.TasteIssue, req.Complaint, req.Equipment, photosJSON, req.Detail, u.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logEvent(id, "开桶反馈", u, req.Detail)

	// 批次口味被多家投诉 → 自动生成品控工单 + 口味异常排查单（批次暂停配送）
	if req.TasteIssue || req.Complaint {
		var bars int
		_ = s.db.QueryRow(`SELECT count(DISTINCT bar_id) FROM keg_feedback
			WHERE batch_id=$1 AND (taste_issue OR complaint)`, batchID).Scan(&bars)
		if bars >= 2 {
			var openCases int
			_ = s.db.QueryRow(`SELECT count(*) FROM cases WHERE batch_id=$1 AND case_type='taste_complaint' AND status <> 'resolved'`, batchID).Scan(&openCases)
			if openCases == 0 {
				title := fmt.Sprintf("批次口味投诉：%s", batchNo)
				detail := fmt.Sprintf("已有 %d 家酒吧反馈批次 %s 口味异常/客诉，请品控评估是否停售或召回。", bars, batchNo)
				_, caseNo, err := s.createCase("taste_complaint", nil, &batchID, nil, title, detail)
				if err == nil {
					s.notify(nil, "qc", nil, nil, "case", "批次口味投诉工单 "+caseNo, detail)
					s.notify(nil, "warehouse", nil, nil, "case", "批次口味投诉工单 "+caseNo, detail)
				}
			}
			trigger := fmt.Sprintf("已有 %d 家酒吧反馈批次 %s 口味异常/客诉（最新：%s）。", bars, batchNo, req.Detail)
			s.openInvestigationForBatch(batchID, trigger, u)
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleKegEmpty(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "bar_manager") {
		writeErr(w, 403, "仅酒吧店长可标记打空")
		return
	}
	id, _ := pathID(r, "id")
	status, _, err := s.kegAtBar(id, u)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if status != "opened" {
		writeErr(w, 400, "仅已开桶的酒桶可标记打空")
		return
	}
	if _, err := s.db.Exec(`UPDATE kegs SET status='empty', emptied_at=now() WHERE id=$1`, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logEvent(id, "打空", u, "酒桶已打空")
	writeJSON(w, 200, map[string]string{"status": "empty"})
}

func (s *Server) handleKegLost(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "bar_manager", "warehouse") {
		writeErr(w, 403, "仅酒吧店长/仓库可登记丢失")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		Note string `json:"note"`
	}
	_ = decodeBody(r, &req)
	var status, kegNo string
	var barID, batchID *int
	err := s.db.QueryRow(`SELECT status, keg_no, current_bar_id, batch_id FROM kegs WHERE id=$1`, id).
		Scan(&status, &kegNo, &barID, &batchID)
	if err != nil {
		writeErr(w, 404, "酒桶不存在")
		return
	}
	if u.Role == "bar_manager" && (u.BarID == nil || barID == nil || *u.BarID != *barID) {
		writeErr(w, 400, "该酒桶不在贵店")
		return
	}
	if status == "lost" {
		writeErr(w, 400, "该酒桶已登记丢失")
		return
	}
	if _, err := s.db.Exec(`UPDATE kegs SET status='lost', refill_eligible=FALSE WHERE id=$1`, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logEvent(id, "登记丢失", u, req.Note)
	title := fmt.Sprintf("酒桶丢失：%s", kegNo)
	detail := fmt.Sprintf("酒桶 %s 登记丢失。%s", kegNo, req.Note)
	_, caseNo, _ := s.createCase("lost", &id, batchID, barID, title, detail)
	s.notify(nil, "finance", nil, nil, "case", "酒桶丢失工单 "+caseNo, detail+" 请关注押金扣减。")
	s.notify(nil, "warehouse", nil, nil, "case", "酒桶丢失工单 "+caseNo, detail)
	writeJSON(w, 200, map[string]string{"status": "lost", "case_no": caseNo})
}

func (s *Server) handleKegRefill(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse") {
		writeErr(w, 403, "仅酒厂仓库可再灌装")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		BatchID int `json:"batch_id"`
	}
	if err := decodeBody(r, &req); err != nil || req.BatchID == 0 {
		writeErr(w, 400, "请选择目标批次")
		return
	}
	var status, kegNo string
	var eligible bool
	err := s.db.QueryRow(`SELECT status, keg_no, refill_eligible FROM kegs WHERE id=$1`, id).Scan(&status, &kegNo, &eligible)
	if err != nil {
		writeErr(w, 404, "酒桶不存在")
		return
	}
	if status != "ready_refill" || !eligible {
		writeErr(w, 400, "仅清洗合格且具备再灌装资格的酒桶可再灌装")
		return
	}
	var batchNo, batchStatus string
	var hold bool
	if err := s.db.QueryRow(`SELECT batch_no, status, delivery_hold FROM batches WHERE id=$1`, req.BatchID).Scan(&batchNo, &batchStatus, &hold); err != nil {
		writeErr(w, 400, "批次不存在")
		return
	}
	if batchStatus != "active" {
		writeErr(w, 400, "目标批次已停售/召回")
		return
	}
	if hold {
		writeErr(w, 400, "目标批次口味排查中，已暂停灌装与配送")
		return
	}
	if _, err := s.db.Exec(`UPDATE kegs SET batch_id=$1, status='in_warehouse', current_bar_id=NULL,
		delivered_at=NULL, opened_at=NULL, emptied_at=NULL, returned_at=NULL WHERE id=$2`, req.BatchID, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logEvent(id, "再灌装入库", u, fmt.Sprintf("再灌装批次 %s，入酒厂仓库", batchNo))
	writeJSON(w, 200, map[string]string{"status": "in_warehouse"})
}

func (s *Server) handleKegWash(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "washer") {
		writeErr(w, 403, "仅清洗厂可登记检测结果")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		Result string `json:"result"` // pass / fail
		Note   string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil || (req.Result != "pass" && req.Result != "fail") {
		writeErr(w, 400, "result 必须为 pass 或 fail")
		return
	}
	var status, kegNo string
	var batchID, barID *int
	err := s.db.QueryRow(`SELECT status, keg_no, batch_id, current_bar_id FROM kegs WHERE id=$1`, id).
		Scan(&status, &kegNo, &batchID, &barID)
	if err != nil {
		writeErr(w, 404, "酒桶不存在")
		return
	}
	if status != "at_wash" && status != "wash_failed" {
		writeErr(w, 400, "仅待清洗/曾不合格的酒桶可登记检测")
		return
	}
	if _, err := s.db.Exec(`INSERT INTO wash_records (keg_id, washer_id, result, note) VALUES ($1,$2,$3,$4)`,
		id, u.ID, req.Result, req.Note); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if req.Result == "pass" {
		_, _ = s.db.Exec(`UPDATE kegs SET status='ready_refill', refill_eligible=TRUE WHERE id=$1`, id)
		s.logEvent(id, "清洗合格", u, req.Note)
		s.autoReleaseDeposit(id, u)
	} else {
		_, _ = s.db.Exec(`UPDATE kegs SET status='wash_failed', refill_eligible=FALSE WHERE id=$1`, id)
		s.logEvent(id, "清洗不合格", u, req.Note)
		title := fmt.Sprintf("清洗检测不合格：%s", kegNo)
		detail := fmt.Sprintf("清洗厂检测：%s；该桶暂不具备再灌装资格。", req.Note)
		_, caseNo, _ := s.createCase("wash_failed", &id, batchID, barID, title, detail)
		s.notify(nil, "qc", nil, nil, "case", "清洗不合格工单 "+caseNo, detail)
		s.notify(nil, "warehouse", nil, nil, "case", "清洗不合格工单 "+caseNo, detail)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
