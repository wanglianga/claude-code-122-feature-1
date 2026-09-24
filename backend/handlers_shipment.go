package main

import (
	"fmt"
	"net/http"
	"strings"
)

// ---------- 配送单 ----------

func (s *Server) handleShipmentList(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if v := r.URL.Query().Get("status"); v != "" {
		add("sh.status = $%d", v)
	}
	switch u.Role {
	case "driver":
		add("sh.driver_id = $%d", u.ID)
	case "bar_manager":
		if u.BarID != nil {
			add("sh.bar_id = $%d", *u.BarID)
		}
	case "dealer":
		if u.DealerID != nil {
			add("sh.dealer_id = $%d", *u.DealerID)
		}
	}
	rows, err := s.db.Query(`
		SELECT sh.id, sh.order_no, COALESCE(d.name,''), COALESCE(b.name,''), COALESCE(dr.name,''),
			sh.status, sh.is_festival, sh.note, sh.receiver_name,
			to_char(sh.created_at,'YYYY-MM-DD HH24:MI'),
			(SELECT count(*) FROM shipment_kegs sk WHERE sk.shipment_id=sh.id)
		FROM shipments sh
		LEFT JOIN dealers d ON d.id=sh.dealer_id
		LEFT JOIN bars b ON b.id=sh.bar_id
		LEFT JOIN users dr ON dr.id=sh.driver_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY sh.id DESC`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, kegCount int
		var orderNo, dealer, bar, driver, status, note, receiver, ts string
		var festival bool
		if err := rows.Scan(&id, &orderNo, &dealer, &bar, &driver, &status, &festival, &note, &receiver, &ts, &kegCount); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "order_no": orderNo, "dealer_name": dealer, "bar_name": bar, "driver_name": driver,
			"status": status, "is_festival": festival, "note": note, "receiver_name": receiver,
			"created_at": ts, "keg_count": kegCount,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleShipmentCreate(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse") {
		writeErr(w, 403, "仅酒厂仓库可创建配送单")
		return
	}
	var req struct {
		BarID      int    `json:"bar_id"`
		DriverID   int    `json:"driver_id"`
		KegIDs     []int  `json:"keg_ids"`
		IsFestival bool   `json:"is_festival"`
		Note       string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil || req.BarID == 0 || req.DriverID == 0 || len(req.KegIDs) == 0 {
		writeErr(w, 400, "酒吧/司机/酒桶均为必填")
		return
	}
	var dealerID *int
	var barName string
	if err := s.db.QueryRow(`SELECT dealer_id, name FROM bars WHERE id=$1`, req.BarID).Scan(&dealerID, &barName); err != nil {
		writeErr(w, 400, "酒吧不存在")
		return
	}
	// 校验酒桶均在库且批次在售
	for _, kid := range req.KegIDs {
		var status, batchStatus string
		err := s.db.QueryRow(`SELECT k.status, COALESCE(b.status,'active') FROM kegs k
			LEFT JOIN batches b ON b.id=k.batch_id WHERE k.id=$1`, kid).Scan(&status, &batchStatus)
		if err != nil {
			writeErr(w, 400, fmt.Sprintf("酒桶 %d 不存在", kid))
			return
		}
		if status != "in_warehouse" {
			writeErr(w, 400, fmt.Sprintf("酒桶 %d 不在仓库，无法出库", kid))
			return
		}
		if batchStatus != "active" {
			writeErr(w, 400, fmt.Sprintf("酒桶 %d 所属批次已停售/召回，禁止出库", kid))
			return
		}
	}
	var shID int
	err := s.db.QueryRow(`INSERT INTO shipments (order_no, dealer_id, bar_id, driver_id, is_festival, note)
		VALUES ('', $1, $2, $3, $4, $5) RETURNING id`,
		nullInt(dealerID), req.BarID, req.DriverID, req.IsFestival, req.Note).Scan(&shID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	orderNo := fmt.Sprintf("SO%06d", shID)
	_, _ = s.db.Exec(`UPDATE shipments SET order_no=$1 WHERE id=$2`, orderNo, shID)
	for _, kid := range req.KegIDs {
		if _, err := s.db.Exec(`INSERT INTO shipment_kegs (shipment_id, keg_id) VALUES ($1,$2)`, shID, kid); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		_, _ = s.db.Exec(`UPDATE kegs SET status='in_transit' WHERE id=$1`, kid)
		s.logEvent(kid, "出库配送", u, fmt.Sprintf("配送单 %s 发往「%s」", orderNo, barName))
	}
	s.notify(&req.DriverID, "", nil, nil, "shipment", "新配送任务 "+orderNo,
		fmt.Sprintf("请将 %d 只酒桶配送至「%s」。", len(req.KegIDs), barName))
	s.notify(nil, "", &req.BarID, nil, "shipment", "发货通知 "+orderNo,
		fmt.Sprintf("酒厂已发出 %d 只酒桶，请留意收货。", len(req.KegIDs)))
	writeJSON(w, 200, map[string]any{"id": shID, "order_no": orderNo})
}

func (s *Server) handleShipmentDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效配送单 ID")
		return
	}
	var orderNo, dealer, bar, driver, status, note, receiver string
	var festival bool
	var created, started, delivered, received *string
	err = s.db.QueryRow(`
		SELECT sh.order_no, COALESCE(d.name,''), COALESCE(b.name,''), COALESCE(dr.name,''),
			sh.status, sh.is_festival, sh.note, sh.receiver_name,
			to_char(sh.created_at,'YYYY-MM-DD HH24:MI'), to_char(sh.started_at,'YYYY-MM-DD HH24:MI'),
			to_char(sh.delivered_at,'YYYY-MM-DD HH24:MI'), to_char(sh.received_at,'YYYY-MM-DD HH24:MI')
		FROM shipments sh
		LEFT JOIN dealers d ON d.id=sh.dealer_id
		LEFT JOIN bars b ON b.id=sh.bar_id
		LEFT JOIN users dr ON dr.id=sh.driver_id
		WHERE sh.id=$1`, id).
		Scan(&orderNo, &dealer, &bar, &driver, &status, &festival, &note, &receiver,
			&created, &started, &delivered, &received)
	if err != nil {
		writeErr(w, 404, "配送单不存在")
		return
	}
	kegs := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT k.id, k.keg_no, k.capacity_liters, COALESCE(b.batch_no,''), k.status, k.deposit_amount
		FROM shipment_kegs sk JOIN kegs k ON k.id=sk.keg_id
		LEFT JOIN batches b ON b.id=k.batch_id
		WHERE sk.shipment_id=$1 ORDER BY k.keg_no`, id)
	if err == nil {
		for rows.Next() {
			var kid, cap int
			var kno, batchNo, kstatus string
			var dep float64
			_ = rows.Scan(&kid, &kno, &cap, &batchNo, &kstatus, &dep)
			kegs = append(kegs, map[string]any{
				"id": kid, "keg_no": kno, "capacity_liters": cap, "batch_no": batchNo,
				"status": kstatus, "deposit_amount": dep,
			})
		}
		rows.Close()
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "order_no": orderNo, "dealer_name": dealer, "bar_name": bar, "driver_name": driver,
		"status": status, "is_festival": festival, "note": note, "receiver_name": receiver,
		"created_at": created, "started_at": started, "delivered_at": delivered, "received_at": received,
		"kegs": kegs,
	})
}

// shipmentOf 校验配送单归属当前司机。
func (s *Server) shipmentOf(id int, u *User) (string, error) {
	var status string
	var driverID *int
	err := s.db.QueryRow(`SELECT status, driver_id FROM shipments WHERE id=$1`, id).Scan(&status, &driverID)
	if err != nil {
		return "", fmt.Errorf("配送单不存在")
	}
	if u.Role == "driver" && (driverID == nil || *driverID != u.ID) {
		return "", fmt.Errorf("该配送单不属于当前司机")
	}
	return status, nil
}

func (s *Server) handleShipmentStart(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "driver", "warehouse") {
		writeErr(w, 403, "仅司机/仓库可操作")
		return
	}
	id, _ := pathID(r, "id")
	status, err := s.shipmentOf(id, u)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if status != "pending" {
		writeErr(w, 400, "仅待配送订单可开始配送")
		return
	}
	_, _ = s.db.Exec(`UPDATE shipments SET status='in_transit', started_at=now() WHERE id=$1`, id)
	writeJSON(w, 200, map[string]string{"status": "in_transit"})
}

func (s *Server) handleShipmentDeliver(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "driver", "warehouse") {
		writeErr(w, 403, "仅司机/仓库可操作")
		return
	}
	id, _ := pathID(r, "id")
	status, err := s.shipmentOf(id, u)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if status != "in_transit" {
		writeErr(w, 400, "仅配送中订单可标记送达")
		return
	}
	_, _ = s.db.Exec(`UPDATE shipments SET status='delivered', delivered_at=now() WHERE id=$1`, id)
	var barID *int
	_ = s.db.QueryRow(`SELECT bar_id FROM shipments WHERE id=$1`, id).Scan(&barID)
	s.notify(nil, "", barID, nil, "shipment", "酒桶已送达", "配送单已送达，请核对酒桶并确认签收。")
	writeJSON(w, 200, map[string]string{"status": "delivered"})
}

func (s *Server) handleShipmentReceive(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "bar_manager") {
		writeErr(w, 403, "仅酒吧店长可签收")
		return
	}
	id, _ := pathID(r, "id")
	var req struct {
		ReceiverName string `json:"receiver_name"`
	}
	if err := decodeBody(r, &req); err != nil || req.ReceiverName == "" {
		writeErr(w, 400, "请填写收货人姓名")
		return
	}
	var status string
	var barID *int
	err := s.db.QueryRow(`SELECT status, bar_id FROM shipments WHERE id=$1`, id).Scan(&status, &barID)
	if err != nil {
		writeErr(w, 404, "配送单不存在")
		return
	}
	if u.Role == "bar_manager" && (u.BarID == nil || barID == nil || *u.BarID != *barID) {
		writeErr(w, 400, "该配送单不属于贵店")
		return
	}
	if status != "delivered" && status != "in_transit" {
		writeErr(w, 400, "仅已送达/配送中的订单可签收")
		return
	}
	if _, err := s.db.Exec(`UPDATE shipments SET status='received', receiver_name=$1, received_at=now() WHERE id=$2`,
		req.ReceiverName, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 酒桶到店：状态变更 + 押金收取
	rows, err := s.db.Query(`SELECT k.id, k.keg_no, k.deposit_amount FROM shipment_kegs sk
		JOIN kegs k ON k.id=sk.keg_id WHERE sk.shipment_id=$1`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type kk struct {
		id  int
		no  string
		dep float64
	}
	var kegs []kk
	for rows.Next() {
		var x kk
		_ = rows.Scan(&x.id, &x.no, &x.dep)
		kegs = append(kegs, x)
	}
	rows.Close()
	for _, x := range kegs {
		_, _ = s.db.Exec(`UPDATE kegs SET status='at_bar', current_bar_id=$1, delivered_at=now() WHERE id=$2`, barID, x.id)
		_, _ = s.db.Exec(`INSERT INTO deposit_ledger (keg_id, bar_id, entry_type, amount, reason, period, status, created_by)
			VALUES ($1,$2,'charge',$3,'酒吧签收，收取押金',$4,'settled',$5)`, x.id, barID, x.dep, periodNow(), u.ID)
		s.logEvent(x.id, "酒吧签收", u, "收货人："+req.ReceiverName)
	}
	var barName string
	_ = s.db.QueryRow(`SELECT COALESCE(name,'') FROM bars WHERE id=$1`, barID).Scan(&barName)
	s.notify(nil, "warehouse", nil, nil, "shipment", "配送单已签收",
		fmt.Sprintf("配送单 %d 已由「%s」签收，收货人 %s。", id, barName, req.ReceiverName))
	s.notify(nil, "finance", nil, nil, "deposit", "押金已收取",
		fmt.Sprintf("配送单 %d 签收，共收取 %d 只酒桶押金（账期 %s）。", id, len(kegs), periodNow()))
	writeJSON(w, 200, map[string]string{"status": "received"})
}

func (s *Server) handleShipmentCancel(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse") {
		writeErr(w, 403, "仅酒厂仓库可取消配送单")
		return
	}
	id, _ := pathID(r, "id")
	var status string
	if err := s.db.QueryRow(`SELECT status FROM shipments WHERE id=$1`, id).Scan(&status); err != nil {
		writeErr(w, 404, "配送单不存在")
		return
	}
	if status != "pending" && status != "held" {
		writeErr(w, 400, "仅待配送/已暂缓的订单可取消")
		return
	}
	_, _ = s.db.Exec(`UPDATE shipments SET status='cancelled' WHERE id=$1`, id)
	rows, err := s.db.Query(`SELECT keg_id FROM shipment_kegs WHERE shipment_id=$1`, id)
	if err == nil {
		var ids []int
		for rows.Next() {
			var kid int
			_ = rows.Scan(&kid)
			ids = append(ids, kid)
		}
		rows.Close()
		for _, kid := range ids {
			_, _ = s.db.Exec(`UPDATE kegs SET status='in_warehouse' WHERE id=$1 AND status='in_transit'`, kid)
			s.logEvent(kid, "配送取消返库", u, fmt.Sprintf("配送单 %d 取消，酒桶返回仓库", id))
		}
	}
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

// ---------- 回收预约 ----------

func (s *Server) handlePickupList(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	where := []string{"1=1"}
	args := []any{}
	if u.Role == "bar_manager" && u.BarID != nil {
		args = append(args, *u.BarID)
		where = append(where, fmt.Sprintf("p.bar_id = $%d", len(args)))
	}
	if v := r.URL.Query().Get("status"); v != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("p.status = $%d", len(args)))
	}
	rows, err := s.db.Query(`
		SELECT p.id, p.pickup_no, COALESCE(b.name,''), p.status, p.expected_count, p.scanned_count,
			p.note, to_char(p.created_at,'YYYY-MM-DD HH24:MI'), COALESCE(to_char(p.completed_at,'YYYY-MM-DD HH24:MI'),'')
		FROM pickups p LEFT JOIN bars b ON b.id=p.bar_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY p.id DESC`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, expected, scanned int
		var no, bar, status, note, ts, completed string
		if err := rows.Scan(&id, &no, &bar, &status, &expected, &scanned, &note, &ts, &completed); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list = append(list, map[string]any{
			"id": id, "pickup_no": no, "bar_name": bar, "status": status,
			"expected_count": expected, "scanned_count": scanned, "note": note,
			"created_at": ts, "completed_at": completed,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handlePickupCreate(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "bar_manager") {
		writeErr(w, 403, "仅酒吧店长可预约回收")
		return
	}
	var req struct {
		KegIDs []int  `json:"keg_ids"`
		Note   string `json:"note"`
	}
	if err := decodeBody(r, &req); err != nil || len(req.KegIDs) == 0 {
		writeErr(w, 400, "请选择要回收的酒桶")
		return
	}
	for _, kid := range req.KegIDs {
		var status string
		var barID *int
		err := s.db.QueryRow(`SELECT status, current_bar_id FROM kegs WHERE id=$1`, kid).Scan(&status, &barID)
		if err != nil {
			writeErr(w, 400, fmt.Sprintf("酒桶 %d 不存在", kid))
			return
		}
		if u.Role == "bar_manager" && (u.BarID == nil || barID == nil || *u.BarID != *barID) {
			writeErr(w, 400, fmt.Sprintf("酒桶 %d 不在贵店", kid))
			return
		}
		if status != "empty" && status != "at_bar" {
			writeErr(w, 400, fmt.Sprintf("酒桶 %d 当前状态不可预约回收（需已打空或未开桶）", kid))
			return
		}
	}
	var pickupID int
	err := s.db.QueryRow(`INSERT INTO pickups (pickup_no, bar_id, note, expected_count) VALUES ('', $1, $2, $3) RETURNING id`,
		u.BarID, req.Note, len(req.KegIDs)).Scan(&pickupID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	pickupNo := fmt.Sprintf("PK%06d", pickupID)
	_, _ = s.db.Exec(`UPDATE pickups SET pickup_no=$1 WHERE id=$2`, pickupNo, pickupID)
	for _, kid := range req.KegIDs {
		_, _ = s.db.Exec(`INSERT INTO pickup_items (pickup_id, keg_id) VALUES ($1,$2)`, pickupID, kid)
		_, _ = s.db.Exec(`UPDATE kegs SET status='pickup_booked' WHERE id=$1`, kid)
		s.logEvent(kid, "预约回收", u, "回收单 "+pickupNo)
	}
	s.notify(nil, "driver", nil, nil, "pickup", "新回收任务 "+pickupNo,
		fmt.Sprintf("酒吧预约回收 %d 只空桶，请前往扫码回收。", len(req.KegIDs)))
	s.notify(nil, "warehouse", nil, nil, "pickup", "新回收预约 "+pickupNo,
		fmt.Sprintf("酒吧预约回收 %d 只空桶。", len(req.KegIDs)))
	writeJSON(w, 200, map[string]any{"id": pickupID, "pickup_no": pickupNo})
}

func (s *Server) handlePickupDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "无效回收单 ID")
		return
	}
	var no, bar, status, note, ts string
	var expected, scanned int
	err = s.db.QueryRow(`
		SELECT p.pickup_no, COALESCE(b.name,''), p.status, p.expected_count, p.scanned_count, p.note,
			to_char(p.created_at,'YYYY-MM-DD HH24:MI')
		FROM pickups p LEFT JOIN bars b ON b.id=p.bar_id WHERE p.id=$1`, id).
		Scan(&no, &bar, &status, &expected, &scanned, &note, &ts)
	if err != nil {
		writeErr(w, 404, "回收单不存在")
		return
	}
	items := []map[string]any{}
	rows, err := s.db.Query(`
		SELECT pi.id, k.keg_no, k.capacity_liters, COALESCE(b.batch_no,''), pi.scanned,
			pi.appearance, pi.residual_liquid, pi.cap_status, COALESCE(to_char(pi.scanned_at,'MM-DD HH24:MI'),'')
		FROM pickup_items pi
		JOIN kegs k ON k.id=pi.keg_id
		LEFT JOIN batches b ON b.id=k.batch_id
		WHERE pi.pickup_id=$1 ORDER BY pi.id`, id)
	if err == nil {
		for rows.Next() {
			var itemID, cap int
			var kegNo, batchNo, appearance, residual, capStatus, scannedAt string
			var scanned bool
			_ = rows.Scan(&itemID, &kegNo, &cap, &batchNo, &scanned, &appearance, &residual, &capStatus, &scannedAt)
			items = append(items, map[string]any{
				"id": itemID, "keg_no": kegNo, "capacity_liters": cap, "batch_no": batchNo,
				"scanned": scanned, "appearance": appearance, "residual_liquid": residual,
				"cap_status": capStatus, "scanned_at": scannedAt,
			})
		}
		rows.Close()
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "pickup_no": no, "bar_name": bar, "status": status,
		"expected_count": expected, "scanned_count": scanned, "note": note, "created_at": ts,
		"items": items,
	})
}

func (s *Server) handlePickupScan(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "driver") {
		writeErr(w, 403, "仅司机可扫码确认")
		return
	}
	pickupID, _ := pathID(r, "id")
	itemID, _ := pathID(r, "itemId")
	var req struct {
		Appearance     string `json:"appearance"`
		ResidualLiquid string `json:"residual_liquid"`
		CapStatus      string `json:"cap_status"`
	}
	if err := decodeBody(r, &req); err != nil || req.Appearance == "" || req.ResidualLiquid == "" || req.CapStatus == "" {
		writeErr(w, 400, "外观/残液/封盖均为必选")
		return
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM pickups WHERE id=$1`, pickupID).Scan(&status); err != nil {
		writeErr(w, 404, "回收单不存在")
		return
	}
	if status != "booked" {
		writeErr(w, 400, "该回收单已完成，无法继续扫码")
		return
	}
	var kegID int
	var scanned bool
	err := s.db.QueryRow(`SELECT keg_id, scanned FROM pickup_items WHERE id=$1 AND pickup_id=$2`, itemID, pickupID).
		Scan(&kegID, &scanned)
	if err != nil {
		writeErr(w, 404, "回收明细不存在")
		return
	}
	if scanned {
		writeErr(w, 400, "该桶已扫码，请勿重复扫描")
		return
	}
	if _, err := s.db.Exec(`UPDATE pickup_items SET scanned=TRUE, appearance=$1, residual_liquid=$2, cap_status=$3,
		scanned_by=$4, scanned_at=now() WHERE id=$5`, req.Appearance, req.ResidualLiquid, req.CapStatus, u.ID, itemID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.db.Exec(`UPDATE pickups SET scanned_count = scanned_count + 1 WHERE id=$1`, pickupID)
	_, _ = s.db.Exec(`UPDATE kegs SET status='in_return' WHERE id=$1`, kegID)
	s.logEvent(kegID, "司机扫码回收", u,
		fmt.Sprintf("外观:%s 残液:%s 封盖:%s", req.Appearance, req.ResidualLiquid, req.CapStatus))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handlePickupComplete(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "driver") {
		writeErr(w, 403, "仅司机可完成回收")
		return
	}
	pickupID, _ := pathID(r, "id")
	var status, pickupNo string
	var barID *int
	err := s.db.QueryRow(`SELECT status, pickup_no, bar_id FROM pickups WHERE id=$1`, pickupID).
		Scan(&status, &pickupNo, &barID)
	if err != nil {
		writeErr(w, 404, "回收单不存在")
		return
	}
	if status != "booked" {
		writeErr(w, 400, "该回收单已完成")
		return
	}
	type item struct {
		id         int
		kegID      int
		kegNo      string
		scanned    bool
		appearance string
		batchID    *int
		delivered  *string
	}
	items := []item{}
	rows, err := s.db.Query(`
		SELECT pi.id, pi.keg_id, k.keg_no, pi.scanned, pi.appearance, k.batch_id,
			to_char(k.delivered_at,'YYYY-MM-DD')
		FROM pickup_items pi JOIN kegs k ON k.id=pi.keg_id WHERE pi.pickup_id=$1`, pickupID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for rows.Next() {
		var it item
		_ = rows.Scan(&it.id, &it.kegID, &it.kegNo, &it.scanned, &it.appearance, &it.batchID, &it.delivered)
		items = append(items, it)
	}
	rows.Close()

	missed := []string{}
	for _, it := range items {
		if !it.scanned {
			missed = append(missed, it.kegNo)
			continue
		}
		// 已扫码 → 送清洗厂
		_, _ = s.db.Exec(`UPDATE kegs SET status='at_wash', returned_at=now() WHERE id=$1`, it.kegID)
		s.logEvent(it.kegID, "回收返厂", u, "空桶回收，送达清洗厂")
		// 外观异常 → 变形/破损工单 + 失去再灌装资格
		if it.appearance != "" && it.appearance != "完好" {
			_, _ = s.db.Exec(`UPDATE kegs SET refill_eligible=FALSE WHERE id=$1`, it.kegID)
			title := fmt.Sprintf("桶体%s：%s", it.appearance, it.kegNo)
			detail := fmt.Sprintf("回收扫码发现桶体%s（回收单 %s），请仓库/财务评估押金扣减。", it.appearance, pickupNo)
			_, caseNo, _ := s.createCase("damaged", &it.kegID, it.batchID, barID, title, detail)
			s.notify(nil, "warehouse", nil, nil, "case", "桶体异常工单 "+caseNo, detail)
			s.notify(nil, "finance", nil, nil, "case", "桶体异常工单 "+caseNo, detail)
		}
	}
	_, _ = s.db.Exec(`UPDATE pickups SET status='completed', completed_at=now(), driver_id=$1 WHERE id=$2`, u.ID, pickupID)

	// 漏扫 → 工单
	if len(missed) > 0 {
		title := fmt.Sprintf("司机漏扫：回收单 %s", pickupNo)
		detail := fmt.Sprintf("回收单 %s 预约 %d 只，实扫 %d 只，漏扫酒桶：%s。请司机/仓库/酒吧核对。",
			pickupNo, len(items), len(items)-len(missed), strings.Join(missed, "、"))
		_, caseNo, _ := s.createCase("missed_scan", nil, nil, barID, title, detail)
		s.notify(&u.ID, "", nil, nil, "case", "漏扫提醒 "+caseNo, detail)
		s.notify(nil, "warehouse", nil, nil, "case", "司机漏扫工单 "+caseNo, detail)
		s.notify(nil, "qc", nil, nil, "case", "司机漏扫工单 "+caseNo, detail)
	}
	// 通知清洗厂
	var atWash int
	_ = s.db.QueryRow(`SELECT count(*) FROM kegs WHERE status='at_wash'`).Scan(&atWash)
	if atWash > 0 {
		s.notify(nil, "washer", nil, nil, "wash", "有待清洗酒桶",
			fmt.Sprintf("当前共 %d 只酒桶待清洗检测。", atWash))
	}
	// 逾期检查
	s.scanLateReturns()
	writeJSON(w, 200, map[string]any{"status": "completed", "missed": missed})
}
