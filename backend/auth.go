package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// User 是登录用户的最小信息集。
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	City     string `json:"city"`
	DealerID *int   `json:"dealer_id"`
	BarID    *int   `json:"bar_id"`
}

type ctxKey string

const ctxUser ctxKey = "user"

// hashPassword 使用加盐 SHA-256（演示系统；生产应换 bcrypt/argon2）。
func hashPassword(pw string) string {
	sum := sha256.Sum256([]byte("kegtrace:" + pw))
	return hex.EncodeToString(sum[:])
}

func genToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if len(h) > 7 && h[:7] == "Bearer " {
			tok := h[7:]
			s.mu.Lock()
			u, ok := s.sessions[tok]
			s.mu.Unlock()
			if ok {
				next(w, r.WithContext(context.WithValue(r.Context(), ctxUser, &u)))
				return
			}
		}
		writeErr(w, http.StatusUnauthorized, "未登录或会话已过期")
	}
}

func userOf(r *http.Request) *User {
	u, _ := r.Context().Value(ctxUser).(*User)
	return u
}

// requireRole 判断用户是否属于给定角色之一；admin 拥有全部权限。
func requireRole(u *User, roles ...string) bool {
	if u == nil {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	for _, r := range roles {
		if u.Role == r {
			return true
		}
	}
	return false
}
