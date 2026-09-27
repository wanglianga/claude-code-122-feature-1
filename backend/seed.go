package main

import (
	"database/sql"
	"fmt"
)

// seed 在空库时写入演示数据（幂等：users 非空则跳过）。
func seed(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	ex := func(q string, args ...any) error {
		_, err := db.Exec(q, args...)
		return err
	}

	// ---------- 经销商 / 酒吧 ----------
	dealers := [][3]string{
		{"京华酒业", "北京", "010-88880001"},
		{"沪上精酿", "上海", "021-66660002"},
		{"羊城酒汇", "广州", "020-55550003"},
	}
	for _, d := range dealers {
		if err := ex(`INSERT INTO dealers (name, city, contact) VALUES ($1,$2,$3)`, d[0], d[1], d[2]); err != nil {
			return err
		}
	}
	bars := []struct {
		name, city string
		dealer     int
		addr, mgr  string
	}{
		{"三里屯野鹅酒吧", "北京", 1, "三里屯北街 12 号", "张三"},
		{"望京麦浪酒吧", "北京", 1, "望京 SOHO T3-105", "李四"},
		{"外滩泡沫实验室", "上海", 2, "中山东一路 27 号", "王五"},
		{"珠江酸啤屋", "广州", 3, "珠江新城花城大道 88 号", "赵六"},
	}
	for _, b := range bars {
		if err := ex(`INSERT INTO bars (name, city, dealer_id, address, manager_name) VALUES ($1,$2,$3,$4,$5)`,
			b.name, b.city, b.dealer, b.addr, b.mgr); err != nil {
			return err
		}
	}

	// ---------- 用户（逐角色演示账号） ----------
	type u struct {
		username, pw, name, role, city string
		dealerID, barID                any
	}
	users := []u{
		{"admin", "admin123", "平台管理员", "admin", "", nil, nil},
		{"warehouse", "warehouse123", "仓库老王", "warehouse", "北京", nil, nil},
		{"bar1", "bar123", "野鹅店长张三", "bar_manager", "北京", nil, 1},
		{"bar2", "bar123", "泡沫店长王五", "bar_manager", "上海", nil, 3},
		{"driver1", "driver123", "司机小李", "driver", "北京", nil, nil},
		{"washer", "washer123", "清洗厂小赵", "washer", "北京", nil, nil},
		{"qc", "qc123", "品控老陈", "qc", "北京", nil, nil},
		{"finance", "finance123", "财务小钱", "finance", "北京", nil, nil},
		{"sales", "sales123", "销售小周", "sales", "北京", nil, nil},
		{"dealer1", "dealer123", "京华酒业对接人", "dealer", "北京", 1, nil},
		{"dealer2", "dealer123", "沪上精酿对接人", "dealer", "上海", 2, nil},
	}
	for _, uu := range users {
		if err := ex(`INSERT INTO users (username, password_hash, name, role, city, dealer_id, bar_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			uu.username, hashPassword(uu.pw), uu.name, uu.role, uu.city, uu.dealerID, uu.barID); err != nil {
			return err
		}
	}
	var driverID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username='driver1'`).Scan(&driverID); err != nil {
		return err
	}

	// ---------- 批次 ----------
	batches := []struct {
		no, style, fill, best string
	}{
		{"IPA-20260901", "IPA", "2026-09-01", "2026-11-30"},
		{"STO-20260905", "世涛", "2026-09-05", "2026-12-04"},
		{"WHT-20260910", "小麦", "2026-09-10", "2026-12-09"},
		{"SOUR-20260912", "酸啤", "2026-09-12", "2026-12-11"},
		{"IPA-20260715", "IPA", "2026-07-15", "2026-10-13"},
	}
	for _, b := range batches {
		if err := ex(`INSERT INTO batches (batch_no, style, fill_date, best_before) VALUES ($1,$2,$3,$4)`,
			b.no, b.style, b.fill, b.best); err != nil {
			return err
		}
	}

	// ---------- 酒桶 ----------
	type k struct {
		no       string
		cap      int
		batch    int
		deposit  float64
		status   string
		bar      any
		eligible bool
	}
	kegs := []k{
		{"KEG-1001", 50, 1, 300, "in_warehouse", nil, true},
		{"KEG-1002", 50, 1, 300, "in_warehouse", nil, true},
		{"KEG-1003", 50, 1, 300, "in_warehouse", nil, true},
		{"KEG-1004", 50, 1, 300, "in_warehouse", nil, true},
		{"KEG-1005", 50, 1, 300, "at_bar", 1, true},
		{"KEG-1006", 50, 1, 300, "at_bar", 1, true},
		{"KEG-1007", 50, 1, 300, "opened", 1, true},
		{"KEG-1008", 50, 1, 300, "empty", 1, true},
		{"KEG-2001", 50, 2, 300, "in_transit", nil, true},
		{"KEG-2002", 50, 2, 300, "in_transit", nil, true},
		{"KEG-2003", 50, 2, 300, "at_bar", 3, true},
		{"KEG-3001", 30, 3, 200, "in_transit", nil, true},
		{"KEG-3002", 30, 3, 200, "in_transit", nil, true},
		{"KEG-4001", 50, 4, 300, "opened", 1, true},
		{"KEG-4002", 50, 4, 300, "opened", 3, true},
		{"KEG-4003", 50, 4, 300, "at_bar", 2, true},
		{"KEG-5001", 50, 5, 300, "at_bar", 2, true},
		{"KEG-5002", 50, 5, 300, "at_wash", 2, true},
		{"KEG-5003", 50, 5, 300, "ready_refill", 2, true},
		{"KEG-5004", 50, 5, 300, "wash_failed", 2, false},
		// 追加在末尾，保持上方既有酒桶的 ID 不变（KEG-1009 = id 21）
		{"KEG-1009", 50, 1, 300, "opened", 3, true},
	}
	for _, kk := range kegs {
		if err := ex(`INSERT INTO kegs (keg_no, capacity_liters, batch_id, deposit_amount, status, current_bar_id, refill_eligible)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			kk.no, kk.cap, kk.batch, kk.deposit, kk.status, kk.bar, kk.eligible); err != nil {
			return err
		}
	}
	// 关键时间戳
	timeSets := []struct{ no, col, expr string }{
		{"KEG-1005", "delivered_at", "now() - interval '14 days'"},
		{"KEG-1006", "delivered_at", "now() - interval '14 days'"},
		{"KEG-1007", "delivered_at", "now() - interval '14 days'"},
		{"KEG-1007", "opened_at", "now() - interval '9 days'"},
		{"KEG-1008", "delivered_at", "now() - interval '14 days'"},
		{"KEG-1008", "opened_at", "now() - interval '12 days'"},
		{"KEG-1008", "emptied_at", "now() - interval '2 days'"},
		{"KEG-1009", "delivered_at", "now() - interval '6 days'"},
		{"KEG-1009", "opened_at", "now() - interval '2 days'"},
		{"KEG-2003", "delivered_at", "now() - interval '10 days'"},
		{"KEG-4001", "delivered_at", "now() - interval '8 days'"},
		{"KEG-4001", "opened_at", "now() - interval '6 days'"},
		{"KEG-4002", "delivered_at", "now() - interval '8 days'"},
		{"KEG-4002", "opened_at", "now() - interval '5 days'"},
		{"KEG-4003", "delivered_at", "now() - interval '8 days'"},
		{"KEG-5001", "delivered_at", "now() - interval '54 days'"},
		{"KEG-5002", "delivered_at", "now() - interval '40 days'"},
		{"KEG-5002", "returned_at", "now() - interval '3 days'"},
		{"KEG-5003", "delivered_at", "now() - interval '45 days'"},
		{"KEG-5003", "returned_at", "now() - interval '10 days'"},
		{"KEG-5004", "delivered_at", "now() - interval '38 days'"},
		{"KEG-5004", "returned_at", "now() - interval '5 days'"},
	}
	for _, ts := range timeSets {
		if err := ex(fmt.Sprintf(`UPDATE kegs SET %s = %s WHERE keg_no=$1`, ts.col, ts.expr), ts.no); err != nil {
			return err
		}
	}

	// ---------- 配送单 ----------
	// SH1 已签收（bar1 的 4 只桶；运输温度 12.5℃，冷链断链嫌疑，供口味排查关联）
	var sh1 int
	if err := db.QueryRow(`INSERT INTO shipments (order_no, dealer_id, bar_id, driver_id, status, receiver_name, transport_temp_c,
			created_at, started_at, delivered_at, received_at)
		VALUES ('', 1, 1, $1, 'received', '张三', 12.5,
			now() - interval '15 days', now() - interval '15 days', now() - interval '14 days', now() - interval '14 days')
		RETURNING id`, driverID).Scan(&sh1); err != nil {
		return err
	}
	if err := ex(`UPDATE shipments SET order_no=$1 WHERE id=$2`, fmt.Sprintf("SO%06d", sh1), sh1); err != nil {
		return err
	}
	for _, no := range []string{"KEG-1005", "KEG-1006", "KEG-1007", "KEG-1008"} {
		if err := ex(`INSERT INTO shipment_kegs (shipment_id, keg_id) SELECT $1, id FROM kegs WHERE keg_no=$2`, sh1, no); err != nil {
			return err
		}
	}
	// SH2 配送中（bar3 的 2 只世涛，冷链正常 4.0℃）
	var sh2 int
	if err := db.QueryRow(`INSERT INTO shipments (order_no, dealer_id, bar_id, driver_id, status, transport_temp_c, created_at, started_at)
		VALUES ('', 2, 3, $1, 'in_transit', 4.0, now() - interval '2 days', now() - interval '1 days') RETURNING id`, driverID).Scan(&sh2); err != nil {
		return err
	}
	if err := ex(`UPDATE shipments SET order_no=$1 WHERE id=$2`, fmt.Sprintf("SO%06d", sh2), sh2); err != nil {
		return err
	}
	for _, no := range []string{"KEG-2001", "KEG-2002"} {
		if err := ex(`INSERT INTO shipment_kegs (shipment_id, keg_id) SELECT $1, id FROM kegs WHERE keg_no=$2`, sh2, no); err != nil {
			return err
		}
	}
	// SH3 待配送·节日临时铺货（bar2 的 2 只小麦）
	var sh3 int
	if err := db.QueryRow(`INSERT INTO shipments (order_no, dealer_id, bar_id, driver_id, status, is_festival, note, created_at)
		VALUES ('', 1, 2, $1, 'pending', TRUE, '中秋国庆临时铺货', now() - interval '1 days') RETURNING id`, driverID).Scan(&sh3); err != nil {
		return err
	}
	if err := ex(`UPDATE shipments SET order_no=$1 WHERE id=$2`, fmt.Sprintf("SO%06d", sh3), sh3); err != nil {
		return err
	}
	for _, no := range []string{"KEG-3001", "KEG-3002"} {
		if err := ex(`INSERT INTO shipment_kegs (shipment_id, keg_id) SELECT $1, id FROM kegs WHERE keg_no=$2`, sh3, no); err != nil {
			return err
		}
	}

	// ---------- 开桶反馈 ----------
	// 演示用客诉照片（内联 SVG data URL，模拟酒吧上传的客诉照片）
	photoIPA := "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxNjAiIGhlaWdodD0iMTAwIj48cmVjdCB3aWR0aD0iMTYwIiBoZWlnaHQ9IjEwMCIgZmlsbD0iIzdjMmQxMiIvPjx0ZXh0IHg9IjEyIiB5PSI0MCIgZmlsbD0iI2ZmZiIgZm9udC1zaXplPSIxNCI+5a6i6K+J54Wn54mHPC90ZXh0Pjx0ZXh0IHg9IjEyIiB5PSI2MiIgZmlsbD0iI2ZkZTY4YSIgZm9udC1zaXplPSIxMiI+SVBBIOawp+WMluWRs8K357q45p2/5ZGzPC90ZXh0Pjwvc3ZnPgo="
	photosIPA := `["` + photoIPA + `"]`
	fb := []struct {
		keg, batch, bar     int
		pour                string
		pressure, taste, cp bool
		equipment           string
		photos              string
		detail              string
	}{
		{7, 1, 1, "偏慢", true, false, false, "酒柱", "", "出酒偏慢，气压表不稳，高峰期跟不上"},
		{7, 1, 1, "正常", false, true, true, "酒柱", photosIPA, "IPA 有氧化味（纸板/湿纸壳味），香气明显衰减，一桌客人退酒"},
		{14, 4, 1, "正常", false, true, true, "酒头直出", photosIPA, "酸啤有纸板味，两桌客人退酒"},
		{15, 4, 3, "正常", false, true, true, "长距离管线", photosIPA, "多名客人反映有哈喇味，已停售该桶"},
	}
	for _, f := range fb {
		if err := ex(`INSERT INTO keg_feedback (keg_id, batch_id, bar_id, pour_speed, pressure_issue, taste_issue, complaint, equipment, photos, detail, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, 1)`, f.keg, f.batch, f.bar, f.pour, f.pressure, f.taste, f.cp, f.equipment, f.photos, f.detail); err != nil {
			return err
		}
	}

	// ---------- 清洗记录 ----------
	if err := ex(`INSERT INTO wash_records (keg_id, washer_id, result, note, created_at)
		VALUES (19, (SELECT id FROM users WHERE username='washer'), 'pass', '内外壁清洗合格，残液已清空', now() - interval '9 days')`); err != nil {
		return err
	}
	if err := ex(`INSERT INTO wash_records (keg_id, washer_id, result, note, created_at)
		VALUES (20, (SELECT id FROM users WHERE username='washer'), 'fail', '内壁残留酒石，需返工', now() - interval '4 days')`); err != nil {
		return err
	}

	// ---------- 押金台账 ----------
	charges := []struct {
		keg, bar int
		amount   float64
		period   string
	}{
		{5, 1, 300, "2026-09"}, {6, 1, 300, "2026-09"}, {7, 1, 300, "2026-09"}, {8, 1, 300, "2026-09"},
		{11, 3, 300, "2026-09"}, {14, 1, 300, "2026-09"}, {15, 3, 300, "2026-09"}, {16, 2, 300, "2026-09"},
		{17, 2, 300, "2026-08"}, {21, 3, 300, "2026-09"},
	}
	for _, c := range charges {
		if err := ex(`INSERT INTO deposit_ledger (keg_id, bar_id, entry_type, amount, reason, period, status)
			VALUES ($1,$2,'charge',$3,'酒吧签收，收取押金',$4,'settled')`, c.keg, c.bar, c.amount, c.period); err != nil {
			return err
		}
	}
	if err := ex(`INSERT INTO deposit_ledger (keg_id, bar_id, entry_type, amount, reason, period, status)
		VALUES (19, 2, 'release', 300, '空桶回收且清洗合格，押金待释放', '2026-09', 'pending')`); err != nil {
		return err
	}

	// ---------- 协同工单 ----------
	if err := ex(`INSERT INTO cases (case_no, case_type, keg_id, batch_id, bar_id, title, detail)
		VALUES ('C000001', 'taste_complaint', NULL, 4, NULL,
			'批次口味投诉：SOUR-20260912',
			'已有 2 家酒吧反馈该批次酸啤口味异常/客诉，请品控评估是否停售或召回。')`); err != nil {
		return err
	}
	if err := ex(`INSERT INTO cases (case_no, case_type, keg_id, batch_id, bar_id, title, detail)
		VALUES ('C000002', 'wash_failed', 20, 5, 2,
			'清洗检测不合格：KEG-5004',
			'清洗厂检测：内壁残留酒石，需返工；该桶暂不具备再灌装资格。')`); err != nil {
		return err
	}
	if err := ex(`INSERT INTO cases (case_no, case_type, keg_id, batch_id, bar_id, title, detail)
		VALUES ('C000003', 'late_return', 17, 5, 2,
			'酒桶逾期未归还：KEG-5001',
			'酒桶 KEG-5001 在酒吧「望京麦浪酒吧」超过 30 天未归还，请仓库/财务/酒吧协同处理。')`); err != nil {
		return err
	}
	if err := ex(`INSERT INTO case_notes (case_id, user_id, role, user_name, note)
		VALUES (1, (SELECT id FROM users WHERE username='qc'), 'qc', '品控老陈', '已取样送检，等待理化指标结果，请仓库暂停该批次出库。')`); err != nil {
		return err
	}

	// ---------- 口味异常排查（酸啤批次已被 2 家酒吧投诉 → 自动排查 + 暂停配送） ----------
	if err := ex(`INSERT INTO investigations (inv_no, batch_id, trigger_detail)
		VALUES ('INV000001', 4, '已有 2 家酒吧反馈该批次酸啤口味异常/客诉（纸板味、哈喇味），品控确认前批次剩余酒桶暂停配送。')`); err != nil {
		return err
	}
	if err := ex(`UPDATE batches SET delivery_hold=TRUE WHERE id=4`); err != nil {
		return err
	}

	// ---------- 通知 ----------
	notifs := []struct {
		role, ntype, title, content string
		barID                       any
	}{
		{"qc", "case", "批次口味投诉工单 C000001", "SOUR-20260912 已被 2 家酒吧投诉口味异常，请尽快评估。", nil},
		{"qc", "investigation", "口味异常排查 INV000001", "SOUR-20260912 已被 2 家酒吧反馈口味异常，批次已暂停配送，请品控尽快给出结论。", nil},
		{"sales", "investigation", "口味异常排查 INV000001", "SOUR-20260912 排查中，批次暂停配送。请销售关注受影响酒吧并准备替换批次。", nil},
		{"warehouse", "investigation", "口味异常排查 INV000001", "SOUR-20260912 排查中，批次剩余酒桶暂停配送，请勿出库。", nil},
		{"washer", "wash", "有待清洗酒桶", "KEG-5002 已送达清洗厂，请及时检测。", nil},
		{"finance", "deposit", "押金待释放：KEG-5003", "KEG-5003 已完成回收并清洗合格，建议释放押金 ¥300.00。", nil},
		{"finance", "case", "逾期归还工单 C000003", "KEG-5001 超过 30 天未归还，请关注押金扣减。", nil},
		{"warehouse", "case", "逾期归还工单 C000003", "KEG-5001 在「望京麦浪酒吧」逾期未归还。", nil},
		{"", "case", "酒桶逾期提醒 C000003", "贵店有酒桶超过 30 天未归还，请尽快预约回收。", 2},
	}
	for _, nf := range notifs {
		if err := ex(`INSERT INTO notifications (role, bar_id, ntype, title, content) VALUES ($1,$2,$3,$4,$5)`,
			nf.role, nf.barID, nf.ntype, nf.title, nf.content); err != nil {
			return err
		}
	}

	// ---------- 酒桶事件时间线 ----------
	events := []struct {
		keg    int
		event  string
		actor  string
		detail string
	}{
		{5, "灌装入库", "仓库老王", "批次 IPA-20260901 灌装，入酒厂仓库"},
		{5, "出库配送", "仓库老王", "配送单发往「三里屯野鹅酒吧」"},
		{5, "酒吧签收", "野鹅店长张三", "收货人：张三"},
		{7, "酒吧签收", "野鹅店长张三", "收货人：张三"},
		{7, "开桶", "野鹅店长张三", "开桶上枪"},
		{7, "开桶反馈", "野鹅店长张三", "出酒偏慢，气压不稳"},
		{7, "开桶反馈", "野鹅店长张三", "IPA 氧化味+客诉"},
		{8, "打空", "野鹅店长张三", "酒桶已打空"},
		{21, "灌装入库", "仓库老王", "批次 IPA-20260901 灌装，入酒厂仓库"},
		{21, "酒吧签收", "泡沫店长王五", "收货人：王五"},
		{21, "开桶", "泡沫店长王五", "开桶上枪"},
		{14, "开桶反馈", "野鹅店长张三", "口味异常+客诉"},
		{15, "开桶反馈", "泡沫店长王五", "口味异常+客诉"},
		{17, "酒吧签收", "望京麦浪酒吧", "收货人：李四"},
		{19, "回收返厂", "司机小李", "空桶回收，送达清洗厂"},
		{19, "清洗合格", "清洗厂小赵", "检测合格，可再灌装"},
		{20, "清洗不合格", "清洗厂小赵", "内壁残留酒石，需返工"},
	}
	for _, e := range events {
		if err := ex(`INSERT INTO keg_events (keg_id, event, actor_name, detail) VALUES ($1,$2,$3,$4)`,
			e.keg, e.event, e.actor, e.detail); err != nil {
			return err
		}
	}

	return nil
}
