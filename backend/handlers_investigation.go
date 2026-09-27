package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

// 运输温度超过该阈值（℃）判定为疑似超温，作为氧化味排查证据。
const transportTempLimit = 8.0

// kegSplit 批次库存拆分行。
type kegSplit struct {
	id     int
	kegNo  string
	status string
	bar    string
}

// kegSplitJSON 序列化库存拆分行。
func kegSplitJSON(list []kegSplit) []map[string]any {
	out := []map[string]any{}
	for _, kr := range list {
		out = append(out, map[string]any{"id": kr.id, "keg_no": kr.kegNo, "status": kr.status, "bar_name": kr.bar})
	}
	return out
}

// classifyBatchKegs 将批次酒桶拆为 已售卖 / 已开桶 / 未开桶 三类。
func (s *Server) classifyBatchKegs(batchID int) (sold, opened, unopened []kegSplit, err error) {
	rows, err := s.db.Query(`
		SELECT k.id, k.keg_no, k.status, COALESCE(b.name,''), k.emptied_at IS NOT NULL
		FROM kegs k LEFT JOIN bars b ON b.id=k.current_bar_id WHERE k.batch_id=$1 ORDER BY k.keg_no`, batchID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kr kegSplit
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
	return sold, opened, unopened, nil
}

// resumeShipmentsForBatch 解除该批次相关配送单的暂缓，回到待配送。
func (s *Server) resumeShipmentsForBatch(batchID int) int {
	res, err := s.db.Exec(`UPDATE shipments SET status='pending'
		WHERE status='held' AND id IN (
			SELECT sk.shipment_id FROM shipment_kegs sk JOIN kegs k ON k.id=sk.keg_id WHERE k.batch_id=$1
		)`, batchID)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// maybeOpenInvestigation 多家酒吧投诉同批次时自动建立排查单并暂停配送（幂等）。
func (s *Server) maybeOpenInvestigation(batchID int) {
	var bars int
	_ = s.db.QueryRow(`SELECT count(DISTINCT bar_id) FROM keg_feedback
		WHERE batch_id=$1 AND (taste_issue OR complaint)`, batchID).Scan(&bars)
	if bars < 2 {
		return
	}
	var open int
	_ = s.db.QueryRow(`SELECT count(*) FROM batch_investigations WHERE batch_id=$1 AND status='investigating'`, batchID).Scan(&open)
	if open > 0 {
		return
	}
	var batchNo, style, status string
	if err := s.db.QueryRow(`SELECT batch_no, style, status FROM batches WHERE id=$1`, batchID).Scan(&batchNo, &style, &status); err != nil {
		return
	}
	if status == "recalled" {
		return
	}
	var invID int
	err := s.db.QueryRow(`INSERT INTO batch_investigations (investigation_no, batch_id) VALUES ('', $1) RETURNING id`, batchID).Scan(&invID)
	if err != nil {
		return
	}
	invNo := fmt.Sprintf("INV%06d", invID)
	_, _ = s.db.Exec(`UPDATE batch_investigations SET investigation_no=$1 WHERE id=$2`, invNo, invID)

	// 品控确认前：批次进入排查暂停态，剩余酒桶暂停配送（在库禁出库 + 在途/待配送单暂缓 + 禁止开桶）
	_, _ = s.db.Exec(`UPDATE batches SET status='investigating',
		suspend_reason='口味异常批次排查中（'||$1||'），暂停配送' WHERE id=$2`, invNo, batchID)
	held := s.holdShipmentsForBatch(batchID)
	barIDs, dealerIDs := s.batchBarsAndDealers(batchID)
	title := "批次口味异常排查 " + invNo
	content := fmt.Sprintf("批次 %s（%s）被 %d 家酒吧反馈口味异常，已建立排查单并暂停配送。"+
		"平台正在关联灌装时间、运输温度、酒吧开桶日期、出酒设备与客诉照片，待品控确认。期间请勿继续开桶售卖。",
		batchNo, style, bars)
	s.notify(nil, "qc", nil, nil, "investigation", title, content)
	s.notify(nil, "warehouse", nil, nil, "investigation", title, content+fmt.Sprintf(" %d 个待配送/配送中订单已暂缓。", held))
	s.notify(nil, "sales", nil, nil, "investigation", title+"：受影响酒吧与可替换批次可查",
		fmt.Sprintf("批次 %s 暂停配送，请到「批次排查」查看受影响酒吧与同酒款可替换批次，提前对接换货。", batchNo))
	for _, barID := range barIDs {
		s.notify(nil, "", &barID, nil, "investigation", title, content)
	}
	for _, dealerID := range dealerIDs {
		s.notify(nil, "", nil, &dealerID, "investigation", title, content+" 请核对经销商库存。")
	}
}

// closeInvestigationIfOpen 批次被直接召回/停售时，联动结束进行中的排查单。
func (s *Server) closeInvestigationIfOpen(batchID int, conclusion, note string) {
	var id int
	if err := s.db.QueryRow(`SELECT id FROM batch_investigations WHERE batch_id=$1 AND status='investigating'`, batchID).Scan(&id); err != nil {
		return
	}
	_, _ = s.db.Exec(`UPDATE batch_investigations SET status='concluded', conclusion=$1, conclusion_note=$2, concluded_at=now() WHERE id=$3`,
		conclusion, note, id)
}

// investigationEvidence 汇总排查证据链：反馈明细 + 灌装/开桶时间 + 送达运输温度 + 设备 + 客诉照片。
func (s *Server) investigationEvidence(batchID int) []map[string]any {
	out := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT f.id, k.keg_no, COALESCE(br.name,''), COALESCE(br.city,''),
			f.pour_speed, f.pressure_issue, f.taste_issue, f.complaint,
			COALESCE(f.equipment,''), COALESCE(f.taste_desc,''), COALESCE(f.photos,''), COALESCE(f.detail,''),
			to_char(b.fill_date,'YYYY-MM-DD'),
			COALESCE(to_char(k.opened_at,'YYYY-MM-DD'),''),
			COALESCE(to_char(ship.delivered_at,'YYYY-MM-DD HH24:MI'),''),
			COALESCE(ship.temp_c::text,''),
			to_char(f.created_at,'YYYY-MM-DD HH24:MI')
		FROM keg_feedback f
		JOIN kegs k ON k.id=f.keg_id
		JOIN batches b ON b.id=f.batch_id
		LEFT JOIN bars br ON br.id=f.bar_id
		LEFT JOIN LATERAL (
			SELECT sh.temp_c, sh.delivered_at FROM shipment_kegs sk2
			JOIN shipments sh ON sh.id=sk2.shipment_id
			WHERE sk2.keg_id=k.id ORDER BY sh.id DESC LIMIT 1
		) ship ON TRUE
		WHERE f.batch_id=$1 ORDER BY f.id DESC`, batchID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var fid int
		var kegNo, bar, city, pour, equipment, tasteDesc, photos, detail string
		var fillDate, openedAt, deliveredAt, tempText, ts string
		var pressure, taste, complaint bool
		if err := rows.Scan(&fid, &kegNo, &bar, &city, &pour, &pressure, &taste, &complaint,
			&equipment, &tasteDesc, &photos, &detail, &fillDate, &openedAt, &deliveredAt, &tempText, &ts); err != nil {
			continue
		}
		overTemp := false
		if tempText != "" {
			var t float64
			if _, e := fmt.Sscanf(tempText, "%f", &t); e == nil && t > transportTempLimit {
				overTemp = true
			}
		}
		photoList := []string{}
		if photos != "" {
			photoList = strings.Split(photos, ",")
		}
		out = append(out, map[string]any{
			"id": fid, "keg_no": kegNo, "bar_name": bar, "city": city,
			"pour_speed": pour, "pressure_issue": pressure, "taste_issue": taste, "complaint": complaint,
			"equipment": equipment, "taste_desc": tasteDesc, "photos": photoList, "detail": detail,
			"fill_date": fillDate, "opened_at": openedAt, "delivered_at": deliveredAt,
			"temp_c": tempText, "over_temp": overTemp, "created_at": ts,
		})
	}
	return out
}

// affectedBars 受影响酒吧（持有该批次酒桶 或 提交过投诉），含在店/已开桶桶数。
func (s *Server) affectedBars(batchID int) []map[string]any {
	out := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT b.id, b.name, b.city, COALESCE(d.name,''),
			(SELECT count(*) FROM kegs k WHERE k.current_bar_id=b.id AND k.batch_id=$1
				AND k.status IN ('at_bar','opened','empty','pickup_booked')),
			(SELECT count(*) FROM kegs k WHERE k.current_bar_id=b.id AND k.batch_id=$1 AND k.status='opened'),
			EXISTS(SELECT 1 FROM keg_feedback f WHERE f.batch_id=$1 AND f.bar_id=b.id AND (f.taste_issue OR f.complaint))
		FROM bars b LEFT JOIN dealers d ON d.id=b.dealer_id
		WHERE b.id IN (
			SELECT current_bar_id FROM kegs WHERE batch_id=$1 AND current_bar_id IS NOT NULL
			UNION SELECT bar_id FROM keg_feedback WHERE batch_id=$1 AND bar_id IS NOT NULL
		) ORDER BY b.id`, batchID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, total, opened int
		var name, city, dealer string
		var complained bool
		if err := rows.Scan(&id, &name, &city, &dealer, &total, &opened, &complained); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "city": city, "dealer_name": dealer,
			"keg_count": total, "opened_count": opened, "complained": complained,
		})
	}
	return out
}

// replacementBatches 同酒款、在售、仓库有现货的可替换批次。
func (s *Server) replacementBatches(style string, excludeBatchID int) []map[string]any {
	out := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT b.id, b.batch_no, to_char(b.fill_date,'YYYY-MM-DD'), to_char(b.best_before,'YYYY-MM-DD'),
			(SELECT count(*) FROM kegs k WHERE k.batch_id=b.id AND k.status='in_warehouse')
		FROM batches b
		WHERE b.style=$1 AND b.status='active' AND b.id<>$2
			AND EXISTS (SELECT 1 FROM kegs k WHERE k.batch_id=b.id AND k.status='in_warehouse')
		ORDER BY b.fill_date DESC, b.id`, style, excludeBatchID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, stock int
		var no, fill, best string
		if err := rows.Scan(&id, &no, &fill, &best, &stock); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "batch_no": no, "fill_date": fill, "best_before": best, "warehouse_stock": stock,
		})
	}
	return out
}

func (s *Server) restockTasks(invID int) []map[string]any {
	out := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT rt.id, rt.bar_id, COALESCE(b.name,''), COALESCE(b.city,''),
			rt.replace_batch_id, COALESCE(bb.batch_no,''), COALESCE(bb.style,''),
			rt.keg_count, rt.status, COALESCE(rt.note,''),
			to_char(rt.created_at,'YYYY-MM-DD HH24:MI'), COALESCE(to_char(rt.done_at,'YYYY-MM-DD HH24:MI'),'')
		FROM restock_tasks rt
		LEFT JOIN bars b ON b.id=rt.bar_id
		LEFT JOIN batches bb ON bb.id=rt.replace_batch_id
		WHERE rt.investigation_id=$1 ORDER BY rt.id`, invID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, barID, kegCount int
		var replaceBatchID sql.NullInt64
		var bar, city, replaceNo, replaceStyle, status, note, created, done string
		if err := rows.Scan(&id, &barID, &bar, &city, &replaceBatchID, &replaceNo, &replaceStyle,
			&kegCount, &status, &note, &created, &done); err != nil {
			continue
		}
		m := map[string]any{
			"id": id, "bar_id": barID, "bar_name": bar, "city": city,
			"replace_batch_id": nil, "replace_batch_no": replaceNo, "replace_style": replaceStyle,
			"keg_count": kegCount, "status": status, "note": note,
			"created_at": created, "done_at": done,
		}
		if replaceBatchID.Valid {
			m["replace_batch_id"] = int(replaceBatchID.Int64)
		}
		out = append(out, m)
	}
	return out
}

func (s *Server) handleInvestigationList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT i.id, i.investigation_no, i.batch_id, b.batch_no, b.style, i.status,
			COALESCE(i.conclusion,''), COALESCE(i.conclusion_note,''),
			to_char(i.created_at,'YYYY-MM-DD HH24:MI'), COALESCE(to_char(i.concluded_at,'YYYY-MM-DD HH24:MI'),''),
			(SELECT count(DISTINCT f.bar_id) FROM keg_feedback f WHERE f.batch_id=i.batch_id AND (f.taste_issue OR f.complaint)),
			(SELECT count(*) FROM restock_tasks rt WHERE rt.investigation_id=i.id),
			(SELECT count(*) FROM restock_tasks rt WHERE rt.investigation_id=i.id AND rt.status<>'done')
		FROM batch_investigations i JOIN batches b ON b.id=i.batch_id
		ORDER BY i.id DESC`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, batchID, complaintBars, taskTotal, taskOpen int
		var invNo, batchNo, style, status, conclusion, note, created, concluded string
		if err := rows.Scan(&id, &invNo, &batchID, &batchNo, &style, &status, &conclusion, &note,
			&created, &concluded, &complaintBars, &taskTotal, &taskOpen); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "investigation_no": invNo, "batch_id": batchID, "batch_no": batchNo, "style": style,
			"status": status, "conclusion": conclusion, "conclusion_note": note,
			"created_at": created, "concluded_at": concluded,
			"complaint_bars": complaintBars, "restock_total": taskTotal, "restock_open": taskOpen,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleInvestigationDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效排查单 ID")
		return
	}
	var batchID, discountRate int
	var invNo, batchNo, style, status, conclusion, note, created string
	var concluded sql.NullString
	err = s.db.QueryRow(`
		SELECT i.investigation_no, i.batch_id, b.batch_no, b.style, i.status,
			COALESCE(i.conclusion,''), COALESCE(i.conclusion_note,''), i.discount_rate,
			to_char(i.created_at,'YYYY-MM-DD HH24:MI'), to_char(i.concluded_at,'YYYY-MM-DD HH24:MI')
		FROM batch_investigations i JOIN batches b ON b.id=i.batch_id WHERE i.id=$1`, id).
		Scan(&invNo, &batchID, &batchNo, &style, &status, &conclusion, &note, &discountRate, &created, &concluded)
	if err != nil {
		writeErr(w, 404, "排查单不存在")
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "investigation_no": invNo, "batch_id": batchID, "batch_no": batchNo, "style": style,
		"status": status, "conclusion": conclusion, "conclusion_note": note, "discount_rate": discountRate,
		"created_at": created, "concluded_at": concluded,
		"evidence":        s.investigationEvidence(batchID),
		"affected_bars":   s.affectedBars(batchID),
		"replacements":    s.replacementBatches(style, batchID),
		"restock_tasks":   s.restockTasks(id),
		"temp_limit":      transportTempLimit,
	})
}

// handleInvestigationConclude 品控结论：recall 召回 / discount 已开桶折价 / clear 排除并解除暂停。
func (s *Server) handleInvestigationConclude(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "qc") {
		writeErr(w, 403, "仅品控可出具排查结论")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		Conclusion   string `json:"conclusion"`
		Note         string `json:"note"`
		DiscountRate int    `json:"discount_rate"`
	}
	if err := decodeBody(r, &req); err != nil || (req.Conclusion != "recall" && req.Conclusion != "discount" && req.Conclusion != "clear") {
		writeErr(w, 400, "结论必须为 recall / discount / clear")
		return
	}
	if req.Note == "" {
		writeErr(w, 400, "请填写品控结论说明")
		return
	}
	var batchID int
	var invNo, batchNo, invStatus string
	err := s.db.QueryRow(`SELECT investigation_no, batch_id, status FROM batch_investigations WHERE id=$1`, id).
		Scan(&invNo, &batchID, &invStatus)
	if err != nil {
		writeErr(w, 404, "排查单不存在")
		return
	}
	if invStatus != "investigating" {
		writeErr(w, 400, "该排查单已出具结论")
		return
	}
	_ = s.db.QueryRow(`SELECT batch_no FROM batches WHERE id=$1`, batchID).Scan(&batchNo)

	resp := map[string]any{"conclusion": req.Conclusion}

	switch req.Conclusion {
	case "recall":
		if _, err := s.db.Exec(`UPDATE batches SET status='recalled', suspend_reason=$1 WHERE id=$2`,
			fmt.Sprintf("排查 %s 确认召回：%s", invNo, req.Note), batchID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		held := s.holdShipmentsForBatch(batchID)
		sold, opened, unopened, _ := s.classifyBatchKegs(batchID)

		// 按受影响酒吧生成仓库换货备货任务；可替换批次保持待销售确认（销售可看到同酒款在售批次）
		createdTasks := 0
		for _, bar := range s.affectedBars(batchID) {
			barID := bar["id"].(int)
			need := bar["opened_count"].(int)
			// 未开桶在店库存同样需要换货
			var unopenedAtBar int
			_ = s.db.QueryRow(`SELECT count(*) FROM kegs WHERE batch_id=$1 AND current_bar_id=$2 AND status='at_bar'`,
				batchID, barID).Scan(&unopenedAtBar)
			need += unopenedAtBar
			if need <= 0 {
				continue
			}
			if _, err := s.db.Exec(`INSERT INTO restock_tasks (investigation_id, bar_id, replace_batch_id, keg_count, note)
				VALUES ($1,$2,NULL,$3,$4)`, id, barID, need,
				fmt.Sprintf("排查 %s 召回换货，待销售确认同酒款替换批次", invNo)); err == nil {
				createdTasks++
			}
		}

		barIDs, dealerIDs := s.batchBarsAndDealers(batchID)
		content := fmt.Sprintf("批次 %s 排查确认召回（%s）。已售卖 %d 桶、已开桶 %d 桶、未开桶 %d 桶；%d 个订单已暂缓，已生成 %d 个酒吧换货备货任务。请立即停售并预约回收。",
			batchNo, req.Note, len(sold), len(opened), len(unopened), held, createdTasks)
		for _, barID := range barIDs {
			s.notify(nil, "", &barID, nil, "batch_recall", "批次召回通知："+batchNo, content)
		}
		for _, dealerID := range dealerIDs {
			s.notify(nil, "", nil, &dealerID, "batch_recall", "批次召回通知："+batchNo, content+" 请核对经销商库存并停止出货。")
		}
		s.notify(nil, "sales", nil, nil, "investigation", "召回换货任务已生成："+batchNo,
			fmt.Sprintf("批次 %s 已确认召回，请在「批次排查」为 %d 家受影响酒吧确认可替换批次并跟进换货。", batchNo, createdTasks))
		s.notify(nil, "warehouse", nil, nil, "batch_recall", "批次召回与备货通知："+batchNo, content)
		s.notify(nil, "finance", nil, nil, "batch_recall", "批次召回通知："+batchNo, content)
		resp["held_shipments"] = held
		resp["restock_tasks"] = createdTasks

	case "discount":
		if req.DiscountRate <= 0 || req.DiscountRate > 100 {
			writeErr(w, 400, "折价比例需在 1~100 之间（%）")
			return
		}
		// 已开桶酒折价：按桶生成折价补偿台账（以待财务结算），未开桶库存恢复配送
		resumed := s.resumeShipmentsForBatch(batchID)
		_, _ = s.db.Exec(`UPDATE batches SET status='active', suspend_reason='' WHERE id=$1`, batchID)
		var discounted int
		rows, err := s.db.Query(`SELECT k.id, k.current_bar_id, k.deposit_amount FROM kegs k
			WHERE k.batch_id=$1 AND k.status='opened' AND k.current_bar_id IS NOT NULL`, batchID)
		if err == nil {
			type dk struct{ id, bar int; amount float64 }
			var list []dk
			for rows.Next() {
				var x dk
				_ = rows.Scan(&x.id, &x.bar, &x.amount)
				list = append(list, x)
			}
			rows.Close()
			for _, x := range list {
				amount := float64(int(x.amount*float64(req.DiscountRate)/100*100+0.5)) / 100
				if amount <= 0 {
					continue
				}
				if _, err := s.db.Exec(`INSERT INTO deposit_ledger (keg_id, bar_id, entry_type, amount, reason, period, status, created_by)
					VALUES ($1,$2,'discount',$3,$4,$5,'pending',$6)`,
					x.id, x.bar, amount,
					fmt.Sprintf("排查 %s 结论：已开桶折价 %d%% 补偿", invNo, req.DiscountRate), periodNow(), u.ID); err == nil {
					discounted++
				}
				s.notify(nil, "", &x.bar, nil, "investigation", "批次折价补偿通知："+batchNo,
					fmt.Sprintf("批次 %s 经品控排查判定按 %d%% 折价处理，贵店在店已开桶将获得折价补偿（财务结算后到账）。", batchNo, req.DiscountRate))
			}
		}
		s.notify(nil, "finance", nil, nil, "deposit", "批次折价补偿待结算："+batchNo,
			fmt.Sprintf("排查 %s 结论折价 %d%%，共 %d 只已开桶生成折价补偿台账，请结算。", invNo, req.DiscountRate, discounted))
		s.notify(nil, "sales", nil, nil, "investigation", "批次折价结论："+batchNo,
			fmt.Sprintf("批次 %s 解除暂停、恢复配送；%d 只已开桶按 %d%% 折价，请向受影响酒吧做好解释。", batchNo, discounted, req.DiscountRate))
		s.notify(nil, "warehouse", nil, nil, "investigation", "批次排查结案："+batchNo,
			fmt.Sprintf("批次 %s 折价结案、恢复配送，%d 个暂缓订单已恢复。", batchNo, resumed))
		resp["resumed_shipments"] = resumed
		resp["discounted_kegs"] = discounted

	case "clear":
		resumed := s.resumeShipmentsForBatch(batchID)
		_, _ = s.db.Exec(`UPDATE batches SET status='active', suspend_reason='' WHERE id=$1`, batchID)
		barIDs, dealerIDs := s.batchBarsAndDealers(batchID)
		content := fmt.Sprintf("批次 %s 经品控排查排除批次质量问题（%s），解除暂停配送，%d 个暂缓订单已恢复。已开桶异常请酒吧结合出酒设备与保存条件自查。",
			batchNo, req.Note, resumed)
		for _, barID := range barIDs {
			s.notify(nil, "", &barID, nil, "investigation", "排查结论：批次恢复配送 "+batchNo, content)
		}
		for _, dealerID := range dealerIDs {
			s.notify(nil, "", nil, &dealerID, "investigation", "排查结论：批次恢复配送 "+batchNo, content)
		}
		s.notify(nil, "qc", nil, nil, "investigation", "排查结案 "+invNo, content)
		s.notify(nil, "warehouse", nil, nil, "investigation", "排查结案 "+invNo, content)
		s.notify(nil, "sales", nil, nil, "investigation", "排查结案 "+invNo, content)
		resp["resumed_shipments"] = resumed
	}

	_, _ = s.db.Exec(`UPDATE batch_investigations
		SET status='concluded', conclusion=$1, conclusion_note=$2, discount_rate=$3, concluded_by=$4, concluded_at=now()
		WHERE id=$5`, req.Conclusion, req.Note, req.DiscountRate, u.ID, id)
	resp["status"] = "concluded"
	writeJSON(w, 200, resp)
}

// handleRestockAssign 销售/仓库为备货任务指定可替换批次与数量。
func (s *Server) handleRestockAssign(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "sales", "warehouse", "qc") {
		writeErr(w, 403, "仅销售/仓库/品控可调整换货备货")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		ReplaceBatchID int `json:"replace_batch_id"`
		KegCount       int `json:"keg_count"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	var status, style string
	var invID int
	err := s.db.QueryRow(`SELECT rt.status, rt.investigation_id, b.style
		FROM restock_tasks rt JOIN batch_investigations i ON i.id=rt.investigation_id
		JOIN batches b ON b.id=i.batch_id WHERE rt.id=$1`, id).Scan(&status, &invID, &style)
	if err != nil {
		writeErr(w, 404, "备货任务不存在")
		return
	}
	if status == "done" {
		writeErr(w, 400, "该备货任务已完成")
		return
	}
	if req.ReplaceBatchID != 0 {
		var bs string
		var bstatus string
		if err := s.db.QueryRow(`SELECT style, status FROM batches WHERE id=$1`, req.ReplaceBatchID).Scan(&bs, &bstatus); err != nil {
			writeErr(w, 400, "替换批次不存在")
			return
		}
		if bs != style || bstatus != "active" {
			writeErr(w, 400, "替换批次必须为同酒款的在售批次")
			return
		}
	}
	if req.KegCount < 0 {
		writeErr(w, 400, "换货数量不能为负")
		return
	}
	if req.KegCount > 0 {
		_, _ = s.db.Exec(`UPDATE restock_tasks SET replace_batch_id=NULLIF($1,0), keg_count=$2 WHERE id=$3`, req.ReplaceBatchID, req.KegCount, id)
	} else {
		_, _ = s.db.Exec(`UPDATE restock_tasks SET replace_batch_id=NULLIF($1,0) WHERE id=$2`, req.ReplaceBatchID, id)
	}
	s.notify(nil, "warehouse", nil, nil, "restock", "换货批次已确认",
		fmt.Sprintf("备货任务 #%d 已指定替换批次（%s），请安排备货出库。", id, style))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// handleRestockAdvance 仓库推进备货状态：open → stocking → done。
func (s *Server) handleRestockAdvance(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse") {
		writeErr(w, 403, "仅酒厂仓库可推进备货")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	_ = decodeBody(r, &req)
	var status string
	var barID, replaceBatchID sql.NullInt64
	var invID int
	err := s.db.QueryRow(`SELECT status, bar_id, replace_batch_id, investigation_id FROM restock_tasks WHERE id=$1`, id).
		Scan(&status, &barID, &replaceBatchID, &invID)
	if err != nil {
		writeErr(w, 404, "备货任务不存在")
		return
	}
	next := req.Status
	if next == "" {
		switch status {
		case "open":
			next = "stocking"
		case "stocking":
			next = "done"
		default:
			writeErr(w, 400, "备货任务已完成")
			return
		}
	}
	if next != "stocking" && next != "done" {
		writeErr(w, 400, "状态只能为 stocking / done")
		return
	}
	if next == "done" && !replaceBatchID.Valid {
		writeErr(w, 400, "请先由销售确认可替换批次再完成备货")
		return
	}
	if next == "done" {
		_, _ = s.db.Exec(`UPDATE restock_tasks SET status='done', note=COALESCE(NULLIF($1,''),note), done_at=now() WHERE id=$2`, req.Note, id)
		if barID.Valid {
			bid := int(barID.Int64)
			s.notify(nil, "", &bid, nil, "restock", "换货酒桶已备好", "贵店召回批次的替换酒桶已由仓库备货完成，等待配送。")
		}
		s.notify(nil, "sales", nil, nil, "restock", "换货备货完成",
			fmt.Sprintf("备货任务 #%d 已完成，可安排配送。", id))
	} else {
		_, _ = s.db.Exec(`UPDATE restock_tasks SET status='stocking', note=COALESCE(NULLIF($1,''),note) WHERE id=$2`, req.Note, id)
	}
	writeJSON(w, 200, map[string]string{"status": next})
}
