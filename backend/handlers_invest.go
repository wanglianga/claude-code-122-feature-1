package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
)

// openInvestigationForBatch 多家酒吧反馈同批次口味异常时自动建立排查单：
// 品控确认前，该批次剩余酒桶进入暂停配送状态（delivery_hold + 暂缓待配送/配送中订单），
// 并通知品控/销售/仓库。已存在排查中的单子时不重复创建。
func (s *Server) openInvestigationForBatch(batchID int, triggerDetail string, actor *User) (bool, string) {
	var open int
	_ = s.db.QueryRow(`SELECT count(*) FROM investigations WHERE batch_id=$1 AND status='investigating'`, batchID).Scan(&open)
	if open > 0 {
		return false, ""
	}
	var batchNo string
	if err := s.db.QueryRow(`SELECT batch_no FROM batches WHERE id=$1`, batchID).Scan(&batchNo); err != nil {
		return false, ""
	}
	var createdBy sql.NullInt64
	if actor != nil {
		createdBy = sql.NullInt64{Int64: int64(actor.ID), Valid: true}
	}
	var id int
	err := s.db.QueryRow(`INSERT INTO investigations (inv_no, batch_id, trigger_detail, created_by)
		VALUES ('', $1, $2, $3) RETURNING id`, batchID, triggerDetail, createdBy).Scan(&id)
	if err != nil {
		return false, ""
	}
	invNo := fmt.Sprintf("INV%06d", id)
	if _, err := s.db.Exec(`UPDATE investigations SET inv_no=$1 WHERE id=$2`, invNo, id); err != nil {
		return false, ""
	}
	// 品控确认前：该批次剩余酒桶暂停配送
	_, _ = s.db.Exec(`UPDATE batches SET delivery_hold=TRUE WHERE id=$1`, batchID)
	held := s.holdShipmentsForBatch(batchID)
	content := fmt.Sprintf("批次 %s 被多家酒吧反馈口味异常，已建立排查单 %s。品控确认前该批次剩余酒桶暂停配送（已暂缓 %d 个待配送/配送中订单）。%s",
		batchNo, invNo, held, triggerDetail)
	s.notify(nil, "qc", nil, nil, "investigation", "口味异常排查 "+invNo, content)
	s.notify(nil, "sales", nil, nil, "investigation", "口味异常排查 "+invNo, content+" 请销售关注受影响酒吧。")
	s.notify(nil, "warehouse", nil, nil, "investigation", "口味异常排查 "+invNo, content)
	return true, invNo
}

// handleInvestList 排查单列表（全角色可见；销售在此查看受影响批次）。
func (s *Server) handleInvestList(w http.ResponseWriter, r *http.Request) {
	where := "1=1"
	args := []any{}
	if v := r.URL.Query().Get("status"); v != "" {
		args = append(args, v)
		where = fmt.Sprintf("i.status = $%d", len(args))
	}
	rows, err := s.db.Query(`
		SELECT i.id, i.inv_no, i.batch_id, COALESCE(b.batch_no,''), COALESCE(b.style,''), COALESCE(b.status,''),
			i.status, i.conclusion, i.trigger_detail,
			to_char(i.created_at,'YYYY-MM-DD HH24:MI'), COALESCE(to_char(i.concluded_at,'YYYY-MM-DD HH24:MI'),''),
			(SELECT count(*) FROM keg_feedback f WHERE f.batch_id=i.batch_id AND (f.taste_issue OR f.complaint)),
			(SELECT count(DISTINCT f.bar_id) FROM keg_feedback f WHERE f.batch_id=i.batch_id AND (f.taste_issue OR f.complaint))
		FROM investigations i LEFT JOIN batches b ON b.id=i.batch_id
		WHERE `+where+` ORDER BY i.id DESC`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, batchID, complaints, complaintBars int
		var invNo, batchNo, style, batchStatus, status, conclusion, trigger, ts, concluded string
		if err := rows.Scan(&id, &invNo, &batchID, &batchNo, &style, &batchStatus, &status, &conclusion, &trigger, &ts, &concluded, &complaints, &complaintBars); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "inv_no": invNo, "batch_id": batchID, "batch_no": batchNo, "style": style,
			"batch_status": batchStatus, "status": status, "conclusion": conclusion,
			"trigger_detail": trigger, "created_at": ts, "concluded_at": concluded,
			"complaints": complaints, "complaint_bars": complaintBars,
		})
	}
	writeJSON(w, 200, list)
}

// handleInvestCreate 品控/仓库手动建立排查单（同样触发暂停配送）。
func (s *Server) handleInvestCreate(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "qc", "warehouse") {
		writeErr(w, 403, "仅品控/仓库可手动建立排查单")
		return
	}
	var req struct {
		BatchID       int    `json:"batch_id"`
		TriggerDetail string `json:"trigger_detail"`
	}
	if err := decodeBody(r, &req); err != nil || req.BatchID == 0 {
		writeErr(w, 400, "请选择批次")
		return
	}
	if req.TriggerDetail == "" {
		req.TriggerDetail = "品控/仓库手动发起口味排查"
	}
	created, invNo := s.openInvestigationForBatch(req.BatchID, req.TriggerDetail, u)
	if !created {
		writeErr(w, 400, "该批次已有排查中的单子，或批次不存在")
		return
	}
	writeJSON(w, 200, map[string]string{"inv_no": invNo})
}

// handleInvestDetail 排查详情：关联灌装时间、运输温度、酒吧开桶日期、出酒设备与客诉照片，
// 并给出受影响酒吧与可替换批次（供销售跟进）。
func (s *Server) handleInvestDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效排查单 ID")
		return
	}
	var (
		invNo, status, trigger, conclusion, conclNote, ts string
		concluded                                         string
		batchID                                           int
		discount                                          *float64
		concludedByName                                   string
	)
	err = s.db.QueryRow(`
		SELECT i.inv_no, i.batch_id, i.status, i.trigger_detail, i.conclusion, i.conclusion_note,
			i.discount_percent, to_char(i.created_at,'YYYY-MM-DD HH24:MI'),
			COALESCE(to_char(i.concluded_at,'YYYY-MM-DD HH24:MI'),''), COALESCE(cu.name,'')
		FROM investigations i LEFT JOIN users cu ON cu.id=i.concluded_by WHERE i.id=$1`, id).
		Scan(&invNo, &batchID, &status, &trigger, &conclusion, &conclNote, &discount, &ts, &concluded, &concludedByName)
	if err != nil {
		writeErr(w, 404, "排查单不存在")
		return
	}
	out := map[string]any{
		"id": id, "inv_no": invNo, "batch_id": batchID, "status": status,
		"trigger_detail": trigger, "conclusion": conclusion, "conclusion_note": conclNote,
		"discount_percent": discount, "created_at": ts, "concluded_at": concluded, "concluded_by": concludedByName,
	}

	// 批次信息（灌装时间）
	var batchNo, style, fill, best, batchStatus, reason string
	var hold bool
	err = s.db.QueryRow(`SELECT batch_no, style, to_char(fill_date,'YYYY-MM-DD'), to_char(best_before,'YYYY-MM-DD'),
		status, suspend_reason, delivery_hold FROM batches WHERE id=$1`, batchID).
		Scan(&batchNo, &style, &fill, &best, &batchStatus, &reason, &hold)
	if err != nil {
		writeErr(w, 404, "关联批次不存在")
		return
	}
	out["batch"] = map[string]any{
		"id": batchID, "batch_no": batchNo, "style": style, "fill_date": fill, "best_before": best,
		"status": batchStatus, "suspend_reason": reason, "delivery_hold": hold,
	}

	// 客诉反馈（出酒设备 + 客诉照片）
	feedbacks := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT f.id, k.keg_no, COALESCE(br.name,''), f.pour_speed, f.pressure_issue, f.taste_issue, f.complaint,
			f.equipment, f.photos, f.detail, to_char(f.created_at,'YYYY-MM-DD HH24:MI')
		FROM keg_feedback f
		JOIN kegs k ON k.id=f.keg_id
		LEFT JOIN bars br ON br.id=f.bar_id
		WHERE f.batch_id=$1 ORDER BY f.id DESC`, batchID)
	if err == nil {
		for rows.Next() {
			var fid int
			var kegNo, barName, pour, equipment, photosJSON, detail, fts string
			var pressure, taste, complaint bool
			_ = rows.Scan(&fid, &kegNo, &barName, &pour, &pressure, &taste, &complaint, &equipment, &photosJSON, &detail, &fts)
			photos := []string{}
			if photosJSON != "" {
				_ = json.Unmarshal([]byte(photosJSON), &photos)
			}
			feedbacks = append(feedbacks, map[string]any{
				"id": fid, "keg_no": kegNo, "bar_name": barName, "pour_speed": pour,
				"pressure_issue": pressure, "taste_issue": taste, "complaint": complaint,
				"equipment": equipment, "photos": photos, "detail": detail, "created_at": fts,
			})
		}
		rows.Close()
	}
	out["feedbacks"] = feedbacks

	// 批次酒桶（酒吧开桶日期）
	kegs := []map[string]any{}
	rows, err = s.db.Query(`
		SELECT k.id, k.keg_no, k.status, COALESCE(br.name,''),
			COALESCE(to_char(k.delivered_at,'YYYY-MM-DD'),''), COALESCE(to_char(k.opened_at,'YYYY-MM-DD'),'')
		FROM kegs k LEFT JOIN bars br ON br.id=k.current_bar_id
		WHERE k.batch_id=$1 ORDER BY k.keg_no`, batchID)
	if err == nil {
		for rows.Next() {
			var kid int
			var kegNo, kstatus, barName, delivered, opened string
			_ = rows.Scan(&kid, &kegNo, &kstatus, &barName, &delivered, &opened)
			kegs = append(kegs, map[string]any{
				"id": kid, "keg_no": kegNo, "status": kstatus, "bar_name": barName,
				"delivered_at": delivered, "opened_at": opened,
			})
		}
		rows.Close()
	}
	out["kegs"] = kegs

	// 运输温度（含该批次酒桶的配送单）
	shipments := []map[string]any{}
	rows, err = s.db.Query(`
		SELECT sh.id, sh.order_no, COALESCE(br.name,''), sh.status, sh.transport_temp_c,
			COALESCE(to_char(sh.started_at,'YYYY-MM-DD HH24:MI'),''), COALESCE(to_char(sh.delivered_at,'YYYY-MM-DD HH24:MI'),''),
			(SELECT count(*) FROM shipment_kegs sk JOIN kegs k2 ON k2.id=sk.keg_id WHERE sk.shipment_id=sh.id AND k2.batch_id=$1)
		FROM shipments sh
		LEFT JOIN bars br ON br.id=sh.bar_id
		WHERE sh.id IN (SELECT sk.shipment_id FROM shipment_kegs sk JOIN kegs k ON k.id=sk.keg_id WHERE k.batch_id=$1)
		ORDER BY sh.id DESC`, batchID)
	if err == nil {
		for rows.Next() {
			var sid, kegCount int
			var orderNo, barName, sstatus, started, delivered string
			var temp *float64
			_ = rows.Scan(&sid, &orderNo, &barName, &sstatus, &temp, &started, &delivered, &kegCount)
			shipments = append(shipments, map[string]any{
				"id": sid, "order_no": orderNo, "bar_name": barName, "status": sstatus,
				"transport_temp_c": temp, "started_at": started, "delivered_at": delivered, "batch_kegs": kegCount,
			})
		}
		rows.Close()
	}
	out["shipments"] = shipments

	// 受影响酒吧（仍持有该批次酒桶的酒吧）
	affected := []map[string]any{}
	rows, err = s.db.Query(`
		SELECT br.id, br.name, br.city, COALESCE(d.name,''),
			count(*) FILTER (WHERE k.status IN ('at_bar','opened')),
			count(*) FILTER (WHERE k.status='opened'),
			count(*) FILTER (WHERE k.status IN ('empty','pickup_booked'))
		FROM kegs k
		JOIN bars br ON br.id=k.current_bar_id
		LEFT JOIN dealers d ON d.id=br.dealer_id
		WHERE k.batch_id=$1 AND k.status IN ('at_bar','opened','empty','pickup_booked')
		GROUP BY br.id, br.name, br.city, d.name ORDER BY br.id`, batchID)
	if err == nil {
		for rows.Next() {
			var barID, sellable, openedN, empties int
			var barName, city, dealerName string
			_ = rows.Scan(&barID, &barName, &city, &dealerName, &sellable, &openedN, &empties)
			affected = append(affected, map[string]any{
				"bar_id": barID, "bar_name": barName, "city": city, "dealer_name": dealerName,
				"in_stock": sellable, "opened": openedN, "emptied": empties,
			})
		}
		rows.Close()
	}
	out["affected_bars"] = affected

	// 可替换批次（同酒款、在售且未暂停配送的其他批次，按在库可发桶数）
	replacements := []map[string]any{}
	rows, err = s.db.Query(`
		SELECT b2.id, b2.batch_no, b2.style, to_char(b2.best_before,'YYYY-MM-DD'),
			(SELECT count(*) FROM kegs k3 WHERE k3.batch_id=b2.id AND k3.status='in_warehouse')
		FROM batches b2
		WHERE b2.style=$1 AND b2.id<>$2 AND b2.status='active' AND NOT b2.delivery_hold
		ORDER BY b2.best_before`, style, batchID)
	if err == nil {
		for rows.Next() {
			var rid, inStock int
			var rno, rstyle, rbest string
			_ = rows.Scan(&rid, &rno, &rstyle, &rbest, &inStock)
			replacements = append(replacements, map[string]any{
				"id": rid, "batch_no": rno, "style": rstyle, "best_before": rbest, "in_warehouse": inStock,
			})
		}
		rows.Close()
	}
	out["replacements"] = replacements
	writeJSON(w, 200, out)
}

// handleInvestConclude 品控结论：recall（召回已开桶及库存）/ discount（已开桶折价）/ release（排除异常恢复配送）。
// 召回与折价会同步销售与仓库备货；解除则恢复该批次配送。
func (s *Server) handleInvestConclude(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "qc") {
		writeErr(w, 403, "仅品控可填写排查结论")
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效排查单 ID")
		return
	}
	var req struct {
		Conclusion      string   `json:"conclusion"` // recall / discount / release
		Note            string   `json:"note"`
		DiscountPercent *float64 `json:"discount_percent"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if req.Conclusion != "recall" && req.Conclusion != "discount" && req.Conclusion != "release" {
		writeErr(w, 400, "结论必须为 recall（召回）/ discount（折价）/ release（解除）")
		return
	}
	if req.Conclusion == "discount" && (req.DiscountPercent == nil || *req.DiscountPercent <= 0 || *req.DiscountPercent > 100) {
		writeErr(w, 400, "折价结论需填写折价比例（0-100，如 30 表示让利 30%）")
		return
	}
	var invNo, status, batchNo string
	var batchID int
	err = s.db.QueryRow(`SELECT i.inv_no, i.status, i.batch_id, COALESCE(b.batch_no,'')
		FROM investigations i LEFT JOIN batches b ON b.id=i.batch_id WHERE i.id=$1`, id).
		Scan(&invNo, &status, &batchID, &batchNo)
	if err != nil {
		writeErr(w, 404, "排查单不存在")
		return
	}
	if status != "investigating" {
		writeErr(w, 400, "该排查单已有结论")
		return
	}

	result := map[string]any{"status": "concluded", "conclusion": req.Conclusion}
	switch req.Conclusion {
	case "recall":
		reason := fmt.Sprintf("品控排查 %s 确认口味异常，召回处理。%s", invNo, req.Note)
		rec, err := s.recallBatch(batchID, reason)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		result["recall"] = rec
	case "discount":
		pct := *req.DiscountPercent
		reason := fmt.Sprintf("品控排查 %s 确认口味异常：已开桶酒折价 %.0f%% 处理，批次停售。%s", invNo, pct, req.Note)
		if _, err := s.db.Exec(`UPDATE batches SET status='suspended', suspend_reason=$1, delivery_hold=FALSE WHERE id=$2`, reason, batchID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		barIDs, dealerIDs := s.batchBarsAndDealers(batchID)
		content := fmt.Sprintf("批次 %s 经品控排查确认口味异常，无需召回：已开桶酒按折价 %.0f%% 与酒吧结算，未开桶库存停止出货。%s",
			batchNo, pct, req.Note)
		for _, barID := range barIDs {
			s.notify(nil, "", &barID, nil, "investigation", "批次折价处理："+batchNo, content)
		}
		for _, dealerID := range dealerIDs {
			s.notify(nil, "", nil, &dealerID, "investigation", "批次折价处理："+batchNo, content+" 请核对经销商库存。")
		}
		s.notify(nil, "sales", nil, nil, "investigation", "批次折价处理："+batchNo, content+" 请销售与受影响酒吧确认折价结算。")
		s.notify(nil, "warehouse", nil, nil, "investigation", "批次折价处理："+batchNo, content+" 仓库停止该批次出货并调整备货。")
		s.notify(nil, "finance", nil, nil, "investigation", "批次折价处理："+batchNo, content+" 请财务关注折价结算差异。")
		result["notified_bars"] = len(barIDs)
		result["discount_percent"] = pct
	case "release":
		if _, err := s.db.Exec(`UPDATE batches SET delivery_hold=FALSE WHERE id=$1`, batchID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		unheld := s.unholdShipmentsForBatch(batchID)
		content := fmt.Sprintf("品控排查 %s 结论：批次 %s 排除口味异常（%s），恢复配送，%d 个暂缓订单已恢复待配送。",
			invNo, batchNo, req.Note, unheld)
		s.notify(nil, "sales", nil, nil, "investigation", "排查解除："+batchNo, content)
		s.notify(nil, "warehouse", nil, nil, "investigation", "排查解除："+batchNo, content+" 可正常出库备货。")
		result["unheld_shipments"] = unheld
	}

	if _, err := s.db.Exec(`UPDATE investigations SET status='concluded', conclusion=$1, conclusion_note=$2,
		discount_percent=$3, concluded_by=$4, concluded_at=now() WHERE id=$5`,
		req.Conclusion, req.Note, nullFloat(req.DiscountPercent), u.ID, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.notify(nil, "qc", nil, nil, "investigation", "排查已结论 "+invNo,
		fmt.Sprintf("排查单 %s（批次 %s）结论：%s。%s", invNo, batchNo, conclusionLabel(req.Conclusion), req.Note))
	writeJSON(w, 200, result)
}

func conclusionLabel(c string) string {
	switch c {
	case "recall":
		return "召回"
	case "discount":
		return "折价"
	case "release":
		return "排除异常，恢复配送"
	}
	return c
}

// nullFloat 把 *float64 转为 sql.NullFloat64。
func nullFloat(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}
