package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func pathID(r *http.Request, name string) (int, error) {
	return strconv.Atoi(r.PathValue(name))
}

// nullInt 把 *int 转为 sql.NullInt64（0/空视为 NULL）。
func nullInt(p *int) sql.NullInt64 {
	if p == nil || *p == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func periodNow() string {
	return time.Now().Format("2006-01")
}

// logEvent 记录酒桶时间线事件。
func (s *Server) logEvent(kegID int, event string, actor *User, detail string) {
	var actorID sql.NullInt64
	actorName := ""
	if actor != nil {
		actorID = sql.NullInt64{Int64: int64(actor.ID), Valid: true}
		actorName = actor.Name
	}
	_, _ = s.db.Exec(`INSERT INTO keg_events (keg_id, event, actor_id, actor_name, detail) VALUES ($1,$2,$3,$4,$5)`,
		kegID, event, actorID, actorName, detail)
}

// notify 发送通知：可定向到具体用户 / 某角色全体 / 某酒吧 / 某经销商。
func (s *Server) notify(userID *int, role string, barID *int, dealerID *int, ntype, title, content string) {
	_, _ = s.db.Exec(`INSERT INTO notifications (user_id, role, bar_id, dealer_id, ntype, title, content)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		nullInt(userID), role, nullInt(barID), nullInt(dealerID), ntype, title, content)
}

// createCase 创建协同工单并返回工单号。
func (s *Server) createCase(caseType string, kegID, batchID, barID *int, title, detail string) (int, string, error) {
	var id int
	err := s.db.QueryRow(`INSERT INTO cases (case_no, case_type, keg_id, batch_id, bar_id, title, detail)
		VALUES ('', $1, $2, $3, $4, $5, $6) RETURNING id`,
		caseType, nullInt(kegID), nullInt(batchID), nullInt(barID), title, detail).Scan(&id)
	if err != nil {
		return 0, "", err
	}
	caseNo := fmt.Sprintf("C%06d", id)
	_, err = s.db.Exec(`UPDATE cases SET case_no=$1 WHERE id=$2`, caseNo, id)
	return id, caseNo, err
}

// hasOpenCase 判断某只桶是否还有未结工单（用于押金自动释放的闸门）。
func (s *Server) hasOpenCase(kegID int) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM cases WHERE keg_id=$1 AND status <> 'resolved'`, kegID).Scan(&n)
	return n > 0
}

// autoReleaseDeposit 酒桶完成"回收+清洗合格"后自动生成待结算的押金释放记录。
func (s *Server) autoReleaseDeposit(kegID int, actor *User) {
	var barID *int
	var deposit float64
	var kegNo string
	err := s.db.QueryRow(`SELECT current_bar_id, deposit_amount, keg_no FROM kegs WHERE id=$1`, kegID).
		Scan(&barID, &deposit, &kegNo)
	if err != nil || barID == nil {
		return
	}
	// 有未结工单（逾期/变形/丢失/清洗不合格等）时不自动释放，交由财务处理
	if s.hasOpenCase(kegID) {
		return
	}
	var pending int
	_ = s.db.QueryRow(`SELECT count(*) FROM deposit_ledger WHERE keg_id=$1 AND entry_type='release' AND status='pending'`, kegID).Scan(&pending)
	if pending > 0 {
		return
	}
	var charged float64
	_ = s.db.QueryRow(`SELECT COALESCE(sum(amount),0) FROM deposit_ledger WHERE keg_id=$1 AND entry_type='charge' AND status='settled'`, kegID).Scan(&charged)
	var released float64
	_ = s.db.QueryRow(`SELECT COALESCE(sum(amount),0) FROM deposit_ledger WHERE keg_id=$1 AND entry_type='release' AND status='settled'`, kegID).Scan(&released)
	amount := charged - released
	if amount <= 0 {
		return
	}
	if amount > deposit {
		amount = deposit
	}
	var creator sql.NullInt64
	if actor != nil {
		creator = sql.NullInt64{Int64: int64(actor.ID), Valid: true}
	}
	_, _ = s.db.Exec(`INSERT INTO deposit_ledger (keg_id, bar_id, entry_type, amount, reason, period, status, created_by)
		VALUES ($1,$2,'release',$3,$4,$5,'pending',$6)`,
		kegID, *barID, amount, "空桶回收且清洗合格，押金待释放", periodNow(), creator)
	s.notify(nil, "finance", nil, nil, "deposit",
		fmt.Sprintf("押金待释放：酒桶 %s", kegNo),
		fmt.Sprintf("酒桶 %s 已完成回收并清洗合格，建议释放押金 ¥%.2f（账期 %s）。", kegNo, amount, periodNow()))
}

// scanLateReturns 扫描逾期未归还的酒桶并生成工单（30 天为限）。
func (s *Server) scanLateReturns() (int, error) {
	rows, err := s.db.Query(`
		SELECT k.id, k.keg_no, k.batch_id, k.current_bar_id, b.name
		FROM kegs k LEFT JOIN bars b ON b.id = k.current_bar_id
		WHERE k.status IN ('at_bar','opened','empty','pickup_booked')
		  AND k.delivered_at IS NOT NULL
		  AND k.delivered_at < now() - interval '30 days'
		  AND NOT EXISTS (
		    SELECT 1 FROM cases c WHERE c.keg_id = k.id AND c.case_type='late_return' AND c.status <> 'resolved'
		  )`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type lateKeg struct {
		id, batchID, barID int
		kegNo, barName     string
	}
	var list []lateKeg
	for rows.Next() {
		var lk lateKeg
		var batchID, barID sql.NullInt64
		var barName sql.NullString
		if err := rows.Scan(&lk.id, &lk.kegNo, &batchID, &barID, &barName); err != nil {
			return 0, err
		}
		if batchID.Valid {
			lk.batchID = int(batchID.Int64)
		}
		if barID.Valid {
			lk.barID = int(barID.Int64)
		}
		lk.barName = barName.String
		list = append(list, lk)
	}
	for _, lk := range list {
		batchID := lk.batchID
		barID := lk.barID
		title := fmt.Sprintf("酒桶逾期未归还：%s", lk.kegNo)
		detail := fmt.Sprintf("酒桶 %s 在酒吧「%s」超过 30 天未归还，请仓库/财务/酒吧协同处理。", lk.kegNo, lk.barName)
		_, caseNo, err := s.createCase("late_return", &lk.id, &batchID, &barID, title, detail)
		if err != nil {
			continue
		}
		s.notify(nil, "warehouse", nil, nil, "case", "逾期归还工单 "+caseNo, detail)
		s.notify(nil, "finance", nil, nil, "case", "逾期归还工单 "+caseNo, detail)
		s.notify(nil, "", &barID, nil, "case", "酒桶逾期提醒 "+caseNo, "贵店有酒桶超过 30 天未归还，请尽快预约回收。")
	}
	return len(list), nil
}
