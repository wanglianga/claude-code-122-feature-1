package main

import (
	"fmt"
	"net/http"
	"strings"
)

// handleDepositList 押金台账；酒吧看本店，经销商看下辖酒吧，其余看全部。
func (s *Server) handleDepositList(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if v := q.Get("period"); v != "" {
		add("d.period = $%d", v)
	}
	if v := q.Get("status"); v != "" {
		add("d.status = $%d", v)
	}
	if v := q.Get("bar_id"); v != "" {
		add("d.bar_id = $%d", v)
	}
	if u.Role == "bar_manager" && u.BarID != nil {
		add("d.bar_id = $%d", *u.BarID)
	}
	if u.Role == "dealer" && u.DealerID != nil {
		add("br.dealer_id = $%d", *u.DealerID)
	}
	rows, err := s.db.Query(`
		SELECT d.id, COALESCE(k.keg_no,''), COALESCE(br.name,''), d.entry_type, d.amount,
			d.reason, d.period, d.status, to_char(d.created_at,'YYYY-MM-DD HH24:MI')
		FROM deposit_ledger d
		LEFT JOIN kegs k ON k.id=d.keg_id
		LEFT JOIN bars br ON br.id=d.bar_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY d.id DESC LIMIT 300`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int
		var kegNo, barName, etype, reason, period, status, ts string
		var amount float64
		if err := rows.Scan(&id, &kegNo, &barName, &etype, &amount, &reason, &period, &status, &ts); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "keg_no": kegNo, "bar_name": barName, "entry_type": etype, "amount": amount,
			"reason": reason, "period": period, "status": status, "created_at": ts,
		})
	}
	writeJSON(w, 200, list)
}

// handleDepositSummary 按账期汇总收取/释放/扣减。
func (s *Server) handleDepositSummary(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT period, entry_type, status, count(*), COALESCE(sum(amount),0)
		FROM deposit_ledger GROUP BY period, entry_type, status ORDER BY period DESC, entry_type, status`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var period, etype, status string
		var cnt int
		var total float64
		if err := rows.Scan(&period, &etype, &status, &cnt, &total); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"period": period, "entry_type": etype, "status": status, "count": cnt, "total": total,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleDepositSettle(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "finance") {
		writeErr(w, 403, "仅财务可结算押金")
		return
	}
	id, _ := pathID(r, "id")
	res, err := s.db.Exec(`UPDATE deposit_ledger SET status='settled' WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeErr(w, 400, "记录不存在或已结算")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "settled"})
}

// handleDepositDeduct 财务手工扣减某只桶的押金。
func (s *Server) handleDepositDeduct(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "finance") {
		writeErr(w, 403, "仅财务可扣减押金")
		return
	}
	var req struct {
		KegID  int     `json:"keg_id"`
		Amount float64 `json:"amount"`
		Reason string  `json:"reason"`
	}
	if err := decodeBody(r, &req); err != nil || req.KegID == 0 || req.Amount <= 0 || req.Reason == "" {
		writeErr(w, 400, "酒桶/金额/原因均为必填")
		return
	}
	var kegNo string
	var barID *int
	if err := s.db.QueryRow(`SELECT keg_no, current_bar_id FROM kegs WHERE id=$1`, req.KegID).Scan(&kegNo, &barID); err != nil {
		writeErr(w, 404, "酒桶不存在")
		return
	}
	if barID == nil {
		writeErr(w, 400, "该酒桶无关联酒吧，无法扣减")
		return
	}
	if _, err := s.db.Exec(`INSERT INTO deposit_ledger (keg_id, bar_id, entry_type, amount, reason, period, status, created_by)
		VALUES ($1,$2,'deduction',$3,$4,$5,'pending',$6)`,
		req.KegID, *barID, req.Amount, req.Reason, periodNow(), u.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.notify(nil, "", barID, nil, "deposit", "押金扣款通知",
		fmt.Sprintf("酒桶 %s 押金将扣减 ¥%.2f：%s", kegNo, req.Amount, req.Reason))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
