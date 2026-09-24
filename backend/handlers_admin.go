package main

import "net/http"

// handleAdminUsers 用户列表（仅管理员）。
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !requireRole(userOf(r), "admin") {
		writeErr(w, 403, "仅管理员")
		return
	}
	rows, err := s.db.Query(`SELECT u.id, u.username, u.name, u.role, u.city,
		COALESCE(d.name,''), COALESCE(b.name,'')
		FROM users u
		LEFT JOIN dealers d ON d.id=u.dealer_id
		LEFT JOIN bars b ON b.id=u.bar_id
		ORDER BY u.id`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int
		var username, name, role, city, dealer, bar string
		_ = rows.Scan(&id, &username, &name, &role, &city, &dealer, &bar)
		list = append(list, map[string]any{
			"id": id, "username": username, "name": name, "role": role, "city": city,
			"dealer_name": dealer, "bar_name": bar,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleAdminUserCreate(w http.ResponseWriter, r *http.Request) {
	if !requireRole(userOf(r), "admin") {
		writeErr(w, 403, "仅管理员")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		City     string `json:"city"`
		DealerID *int   `json:"dealer_id"`
		BarID    *int   `json:"bar_id"`
	}
	if err := decodeBody(r, &req); err != nil || req.Username == "" || req.Password == "" || req.Name == "" || req.Role == "" {
		writeErr(w, 400, "用户名/密码/姓名/角色均为必填")
		return
	}
	var id int
	err := s.db.QueryRow(`INSERT INTO users (username, password_hash, name, role, city, dealer_id, bar_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		req.Username, hashPassword(req.Password), req.Name, req.Role, req.City, nullInt(req.DealerID), nullInt(req.BarID)).Scan(&id)
	if err != nil {
		writeErr(w, 400, "创建失败（用户名可能重复）："+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) handleAdminDealers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, city, contact FROM dealers ORDER BY id`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int
		var name, city, contact string
		_ = rows.Scan(&id, &name, &city, &contact)
		list = append(list, map[string]any{"id": id, "name": name, "city": city, "contact": contact})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleAdminDealerCreate(w http.ResponseWriter, r *http.Request) {
	if !requireRole(userOf(r), "admin") {
		writeErr(w, 403, "仅管理员")
		return
	}
	var req struct {
		Name    string `json:"name"`
		City    string `json:"city"`
		Contact string `json:"contact"`
	}
	if err := decodeBody(r, &req); err != nil || req.Name == "" || req.City == "" {
		writeErr(w, 400, "名称/城市为必填")
		return
	}
	var id int
	if err := s.db.QueryRow(`INSERT INTO dealers (name, city, contact) VALUES ($1,$2,$3) RETURNING id`,
		req.Name, req.City, req.Contact).Scan(&id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) handleAdminBars(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT b.id, b.name, b.city, COALESCE(d.name,''), b.address, b.manager_name
		FROM bars b LEFT JOIN dealers d ON d.id=b.dealer_id ORDER BY b.id`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int
		var name, city, dealer, addr, mgr string
		_ = rows.Scan(&id, &name, &city, &dealer, &addr, &mgr)
		list = append(list, map[string]any{
			"id": id, "name": name, "city": city, "dealer_name": dealer, "address": addr, "manager_name": mgr,
		})
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleAdminBarCreate(w http.ResponseWriter, r *http.Request) {
	if !requireRole(userOf(r), "admin") {
		writeErr(w, 403, "仅管理员")
		return
	}
	var req struct {
		Name     string `json:"name"`
		City     string `json:"city"`
		DealerID int    `json:"dealer_id"`
		Address  string `json:"address"`
		Manager  string `json:"manager_name"`
	}
	if err := decodeBody(r, &req); err != nil || req.Name == "" || req.City == "" || req.DealerID == 0 {
		writeErr(w, 400, "名称/城市/经销商为必填")
		return
	}
	var id int
	if err := s.db.QueryRow(`INSERT INTO bars (name, city, dealer_id, address, manager_name) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		req.Name, req.City, req.DealerID, req.Address, req.Manager).Scan(&id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}
