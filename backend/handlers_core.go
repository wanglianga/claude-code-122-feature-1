package main

import (
	"net/http"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	var u User
	err := s.db.QueryRow(`SELECT id, username, name, role, city, dealer_id, bar_id
		FROM users WHERE username=$1 AND password_hash=$2`,
		req.Username, hashPassword(req.Password)).
		Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.City, &u.DealerID, &u.BarID)
	if err != nil {
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	tok := genToken()
	s.mu.Lock()
	s.sessions[tok] = u
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, userOf(r))
}

// notifWhere 构造当前用户的通知过滤条件。
func notifWhere(u *User) (string, []any) {
	barID, dealerID := 0, 0
	if u.BarID != nil {
		barID = *u.BarID
	}
	if u.DealerID != nil {
		dealerID = *u.DealerID
	}
	return `(user_id = $1 OR (user_id IS NULL AND (role = $2 OR bar_id = NULLIF($3,0) OR dealer_id = NULLIF($4,0))))`,
		[]any{u.ID, u.Role, barID, dealerID}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	out := map[string]any{}

	kegStatus := []map[string]any{}
	rows, err := s.db.Query(`SELECT status, count(*) FROM kegs GROUP BY status ORDER BY 1`)
	if err == nil {
		for rows.Next() {
			var st string
			var c int
			_ = rows.Scan(&st, &c)
			kegStatus = append(kegStatus, map[string]any{"status": st, "count": c})
		}
		rows.Close()
	}
	out["keg_status"] = kegStatus

	var openCases, pendingDeposits, activeBatches, totalKegs, atBar int
	_ = s.db.QueryRow(`SELECT count(*) FROM cases WHERE status <> 'resolved'`).Scan(&openCases)
	_ = s.db.QueryRow(`SELECT count(*) FROM deposit_ledger WHERE status='pending'`).Scan(&pendingDeposits)
	_ = s.db.QueryRow(`SELECT count(*) FROM batches WHERE status='active'`).Scan(&activeBatches)
	_ = s.db.QueryRow(`SELECT count(*) FROM kegs`).Scan(&totalKegs)
	_ = s.db.QueryRow(`SELECT count(*) FROM kegs WHERE status IN ('at_bar','opened')`).Scan(&atBar)
	out["open_cases"] = openCases
	out["pending_deposits"] = pendingDeposits
	out["active_batches"] = activeBatches
	out["total_kegs"] = totalKegs
	out["at_bar_kegs"] = atBar

	where, args := notifWhere(u)
	var unread int
	_ = s.db.QueryRow(`SELECT count(*) FROM notifications WHERE `+where+` AND NOT read`, args...).Scan(&unread)
	out["unread"] = unread

	events := []map[string]any{}
	erows, err := s.db.Query(`
		SELECT e.id, k.keg_no, e.event, e.actor_name, e.detail, to_char(e.created_at,'MM-DD HH24:MI')
		FROM keg_events e JOIN kegs k ON k.id = e.keg_id
		ORDER BY e.id DESC LIMIT 12`)
	if err == nil {
		for erows.Next() {
			var id int
			var kegNo, ev, actor, detail, ts string
			_ = erows.Scan(&id, &kegNo, &ev, &actor, &detail, &ts)
			events = append(events, map[string]any{
				"id": id, "keg_no": kegNo, "event": ev, "actor": actor, "detail": detail, "time": ts,
			})
		}
		erows.Close()
	}
	out["recent_events"] = events
	writeJSON(w, 200, out)
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}

	dealers := []map[string]any{}
	rows, err := s.db.Query(`SELECT id, name, city FROM dealers ORDER BY id`)
	if err == nil {
		for rows.Next() {
			var id int
			var name, city string
			_ = rows.Scan(&id, &name, &city)
			dealers = append(dealers, map[string]any{"id": id, "name": name, "city": city})
		}
		rows.Close()
	}
	out["dealers"] = dealers

	bars := []map[string]any{}
	rows, err = s.db.Query(`SELECT b.id, b.name, b.city, b.dealer_id, COALESCE(d.name,'') FROM bars b
		LEFT JOIN dealers d ON d.id=b.dealer_id ORDER BY b.id`)
	if err == nil {
		for rows.Next() {
			var id, dealerID int
			var name, city, dealerName string
			var dID *int
			_ = rows.Scan(&id, &name, &city, &dID, &dealerName)
			if dID != nil {
				dealerID = *dID
			}
			bars = append(bars, map[string]any{"id": id, "name": name, "city": city, "dealer_id": dealerID, "dealer_name": dealerName})
		}
		rows.Close()
	}
	out["bars"] = bars

	drivers := []map[string]any{}
	rows, err = s.db.Query(`SELECT id, name FROM users WHERE role='driver' ORDER BY id`)
	if err == nil {
		for rows.Next() {
			var id int
			var name string
			_ = rows.Scan(&id, &name)
			drivers = append(drivers, map[string]any{"id": id, "name": name})
		}
		rows.Close()
	}
	out["drivers"] = drivers

	batches := []map[string]any{}
	rows, err = s.db.Query(`SELECT id, batch_no, style, status FROM batches ORDER BY id DESC`)
	if err == nil {
		for rows.Next() {
			var id int
			var no, style, status string
			_ = rows.Scan(&id, &no, &style, &status)
			batches = append(batches, map[string]any{"id": id, "batch_no": no, "style": style, "status": status})
		}
		rows.Close()
	}
	out["batches"] = batches

	out["styles"] = []string{"IPA", "世涛", "小麦", "酸啤"}
	writeJSON(w, 200, out)
}

func (s *Server) handleNotificationList(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	where, args := notifWhere(u)
	rows, err := s.db.Query(`SELECT id, ntype, title, content, read, to_char(created_at,'YYYY-MM-DD HH24:MI')
		FROM notifications WHERE `+where+` ORDER BY id DESC LIMIT 100`, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int
		var ntype, title, content, ts string
		var read bool
		_ = rows.Scan(&id, &ntype, &title, &content, &read, &ts)
		list = append(list, map[string]any{
			"id": id, "ntype": ntype, "title": title, "content": content, "read": read, "created_at": ts,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleNotificationRead(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	where, args := notifWhere(u)
	_, err := s.db.Exec(`UPDATE notifications SET read=TRUE WHERE `+where, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleLateReturnCheck(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if !requireRole(u, "warehouse", "finance") {
		writeErr(w, 403, "仅仓库/财务可触发逾期扫描")
		return
	}
	n, err := s.scanLateReturns()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"created": n})
}
