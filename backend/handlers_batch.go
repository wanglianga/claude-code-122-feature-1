package main

import (
	"fmt"
	"net/http"
)

// holdShipmentsForBatch 暂停包含该批次酒桶的待配送/配送中订单，返回受影响订单数。
func (s *Server) holdShipmentsForBatch(batchID int) int {
	res, err := s.db.Exec(`UPDATE shipments SET status='held'
		WHERE status IN ('pending','in_transit') AND id IN (
			SELECT sk.shipment_id FROM shipment_kegs sk JOIN kegs k ON k.id=sk.keg_id WHERE k.batch_id=$1
		)`, batchID)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

func (s *Server) handleBatchList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT b.id, b.batch_no, b.style, to_char(b.fill_date,'YYYY-MM-DD'), to_char(b.best_before,'YYYY-MM-DD'),
			b.status, b.suspend_reason,
			(SELECT count(*) FROM kegs k WHERE k.batch_id=b.id),
			(SELECT count(*) FROM keg_feedback f WHERE f.batch_id=b.id AND (f.taste_issue OR f.complaint)),
			(SELECT count(DISTINCT f.bar_id) FROM keg_feedback f WHERE f.batch_id=b.id AND (f.taste_issue OR f.complaint))
		FROM batches b ORDER BY b.id DESC`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, kegTotal, complaints, complaintBars int
		var no, style, fill, best, status, reason string
		if err := rows.Scan(&id, &no, &style, &fill, &best, &status, &reason, &kegTotal, &complaints, &complaintBars); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "batch_no": no, "style": style, "fill_date": fill, "best_before": best,
			"status": status, "suspend_reason": reason, "keg_total": kegTotal,
			"complaints": complaints, "complaint_bars": complaintBars,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse") {
		writeErr(w, 403, "仅酒厂仓库可创建批次")
		return
	}
	var req struct {
		BatchNo    string `json:"batch_no"`
		Style      string `json:"style"`
		FillDate   string `json:"fill_date"`
		BestBefore string `json:"best_before"`
	}
	if err := decodeBody(r, &req); err != nil || req.BatchNo == "" || req.Style == "" || req.FillDate == "" || req.BestBefore == "" {
		writeErr(w, 400, "批次号/酒款/灌装日期/最佳饮用期均为必填")
		return
	}
	var id int
	err := s.db.QueryRow(`INSERT INTO batches (batch_no, style, fill_date, best_before) VALUES ($1,$2,$3,$4) RETURNING id`,
		req.BatchNo, req.Style, req.FillDate, req.BestBefore).Scan(&id)
	if err != nil {
		writeErr(w, 400, "创建失败（批次号可能重复）："+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) handleBatchDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效批次 ID")
		return
	}
	var b map[string]any
	var no, style, fill, best, status, reason string
	err = s.db.QueryRow(`SELECT batch_no, style, to_char(fill_date,'YYYY-MM-DD'), to_char(best_before,'YYYY-MM-DD'), status, suspend_reason
		FROM batches WHERE id=$1`, id).Scan(&no, &style, &fill, &best, &status, &reason)
	if err != nil {
		writeErr(w, 404, "批次不存在")
		return
	}
	b = map[string]any{"id": id, "batch_no": no, "style": style, "fill_date": fill, "best_before": best,
		"status": status, "suspend_reason": reason}

	kegs := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT k.id, k.keg_no, k.capacity_liters, k.status, COALESCE(br.name,''), k.deposit_amount, k.refill_eligible
		FROM kegs k LEFT JOIN bars br ON br.id=k.current_bar_id
		WHERE k.batch_id=$1 ORDER BY k.keg_no`, id)
	if err == nil {
		for rows.Next() {
			var kid, cap int
			var kno, st, barName string
			var dep float64
			var eligible bool
			_ = rows.Scan(&kid, &kno, &cap, &st, &barName, &dep, &eligible)
			kegs = append(kegs, map[string]any{
				"id": kid, "keg_no": kno, "capacity_liters": cap, "status": st, "bar_name": barName,
				"deposit_amount": dep, "refill_eligible": eligible,
			})
		}
		rows.Close()
	}
	b["kegs"] = kegs

	feedbacks := []map[string]any{}
	rows, err = s.db.Query(`
		SELECT f.id, k.keg_no, COALESCE(br.name,''), f.pour_speed, f.pressure_issue, f.taste_issue, f.complaint,
			f.detail, to_char(f.created_at,'YYYY-MM-DD HH24:MI')
		FROM keg_feedback f
		JOIN kegs k ON k.id=f.keg_id
		LEFT JOIN bars br ON br.id=f.bar_id
		WHERE f.batch_id=$1 ORDER BY f.id DESC`, id)
	if err == nil {
		for rows.Next() {
			var fid int
			var kno, barName, pour, detail, ts string
			var pressure, taste, complaint bool
			_ = rows.Scan(&fid, &kno, &barName, &pour, &pressure, &taste, &complaint, &detail, &ts)
			feedbacks = append(feedbacks, map[string]any{
				"id": fid, "keg_no": kno, "bar_name": barName, "pour_speed": pour,
				"pressure_issue": pressure, "taste_issue": taste, "complaint": complaint,
				"detail": detail, "created_at": ts,
			})
		}
		rows.Close()
	}
	b["feedbacks"] = feedbacks

	cases := []map[string]any{}
	rows, err = s.db.Query(`SELECT id, case_no, case_type, status, title FROM cases WHERE batch_id=$1 ORDER BY id DESC`, id)
	if err == nil {
		for rows.Next() {
			var cid int
			var cno, ctype, cstatus, title string
			_ = rows.Scan(&cid, &cno, &ctype, &cstatus, &title)
			cases = append(cases, map[string]any{"id": cid, "case_no": cno, "case_type": ctype, "status": cstatus, "title": title})
		}
		rows.Close()
	}
	b["cases"] = cases
	writeJSON(w, 200, b)
}

// batchBarsAndDealers 找出持有该批次酒桶（在酒吧/已开桶）的酒吧与对应经销商。
func (s *Server) batchBarsAndDealers(batchID int) (barIDs []int, dealerIDs []int) {
	rows, err := s.db.Query(`
		SELECT DISTINCT k.current_bar_id, b.dealer_id
		FROM kegs k JOIN bars b ON b.id = k.current_bar_id
		WHERE k.batch_id=$1 AND k.status IN ('at_bar','opened','empty','pickup_booked')`, batchID)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	seenB, seenD := map[int]bool{}, map[int]bool{}
	for rows.Next() {
		var barID int
		var dealerID *int
		if err := rows.Scan(&barID, &dealerID); err != nil {
			continue
		}
		if !seenB[barID] {
			seenB[barID] = true
			barIDs = append(barIDs, barID)
		}
		if dealerID != nil && !seenD[*dealerID] {
			seenD[*dealerID] = true
			dealerIDs = append(dealerIDs, *dealerID)
		}
	}
	return
}

func (s *Server) handleBatchSuspend(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "qc", "warehouse") {
		writeErr(w, 403, "仅品控/仓库可暂停批次销售")
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效批次 ID")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &req)
	if req.Reason == "" {
		req.Reason = "品控评估暂停销售"
	}
	var batchNo string
	err = s.db.QueryRow(`SELECT batch_no FROM batches WHERE id=$1 AND status='active'`, id).Scan(&batchNo)
	if err != nil {
		writeErr(w, 400, "批次不存在或已非在售状态")
		return
	}
	if _, err := s.db.Exec(`UPDATE batches SET status='suspended', suspend_reason=$1 WHERE id=$2`, req.Reason, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	held := s.holdShipmentsForBatch(id)
	barIDs, dealerIDs := s.batchBarsAndDealers(id)
	content := fmt.Sprintf("批次 %s 已暂停销售（%s）。请立即停止开桶售卖；%d 个待配送/配送中订单已暂缓。", batchNo, req.Reason, held)
	for _, barID := range barIDs {
		s.notify(nil, "", &barID, nil, "batch_suspend", "批次停售通知："+batchNo, content)
	}
	for _, dealerID := range dealerIDs {
		s.notify(nil, "", nil, &dealerID, "batch_suspend", "批次停售通知："+batchNo, content+" 请核对经销商库存。")
	}
	s.notify(nil, "warehouse", nil, nil, "batch_suspend", "批次停售通知："+batchNo, content)
	s.notify(nil, "finance", nil, nil, "batch_suspend", "批次停售通知："+batchNo, content)
	writeJSON(w, 200, map[string]any{"status": "suspended", "held_shipments": held, "notified_bars": len(barIDs), "notified_dealers": len(dealerIDs)})
}

func (s *Server) handleBatchRecall(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "qc") {
		writeErr(w, 403, "仅品控可发起批次召回")
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效批次 ID")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &req)
	if req.Reason == "" {
		req.Reason = "批次质量召回"
	}
	var batchNo string
	err = s.db.QueryRow(`SELECT batch_no FROM batches WHERE id=$1 AND status <> 'recalled'`, id).Scan(&batchNo)
	if err != nil {
		writeErr(w, 400, "批次不存在或已召回")
		return
	}
	if _, err := s.db.Exec(`UPDATE batches SET status='recalled', suspend_reason=$1 WHERE id=$2`, req.Reason, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	held := s.holdShipmentsForBatch(id)

	// 库存拆分：已售卖 / 已开桶 / 未开桶
	type kegRow struct {
		id     int
		kegNo  string
		status string
		bar    string
	}
	sold, opened, unopened := []kegRow{}, []kegRow{}, []kegRow{}
	rows, err := s.db.Query(`
		SELECT k.id, k.keg_no, k.status, COALESCE(b.name,''), k.emptied_at IS NOT NULL
		FROM kegs k LEFT JOIN bars b ON b.id=k.current_bar_id WHERE k.batch_id=$1 ORDER BY k.keg_no`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var kr kegRow
			var emptied bool
			if err := rows.Scan(&kr.id, &kr.kegNo, &kr.status, &kr.bar, &emptied); err != nil {
				continue
			}
			switch {
			case emptied || kr.status == "empty" || kr.status == "pickup_booked" || kr.status == "in_return" ||
				kr.status == "at_wash" || kr.status == "ready_refill" || kr.status == "wash_failed" || kr.status == "lost":
				sold = append(sold, kr)
			case kr.status == "opened":
				opened = append(opened, kr)
			default:
				unopened = append(unopened, kr)
			}
		}
	}

	barIDs, dealerIDs := s.batchBarsAndDealers(id)
	content := fmt.Sprintf("批次 %s 已启动召回（%s）。已售卖 %d 桶、已开桶 %d 桶、未开桶 %d 桶；%d 个待配送/配送中订单已暂缓。已开桶/未开桶请立即停售并预约回收。",
		batchNo, req.Reason, len(sold), len(opened), len(unopened), held)
	for _, barID := range barIDs {
		s.notify(nil, "", &barID, nil, "batch_recall", "批次召回通知："+batchNo, content)
	}
	for _, dealerID := range dealerIDs {
		s.notify(nil, "", nil, &dealerID, "batch_recall", "批次召回通知："+batchNo, content+" 请核对经销商库存并停止出货。")
	}
	s.notify(nil, "warehouse", nil, nil, "batch_recall", "批次召回通知："+batchNo, content)
	s.notify(nil, "finance", nil, nil, "batch_recall", "批次召回通知："+batchNo, content)

	toJSON := func(list []kegRow) []map[string]any {
		out := []map[string]any{}
		for _, kr := range list {
			out = append(out, map[string]any{"id": kr.id, "keg_no": kr.kegNo, "status": kr.status, "bar_name": kr.bar})
		}
		return out
	}
	writeJSON(w, 200, map[string]any{
		"status": "recalled", "held_shipments": held,
		"sold": toJSON(sold), "opened": toJSON(opened), "unopened": toJSON(unopened),
		"notified_bars": len(barIDs), "notified_dealers": len(dealerIDs),
	})
}
