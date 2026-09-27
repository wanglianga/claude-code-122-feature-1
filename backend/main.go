package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// Server 持有数据库连接与内存会话。
type Server struct {
	db       *sql.DB
	mu       sync.Mutex
	sessions map[string]User
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://keg:kegpass@localhost:5432/kegdb?sslmode=disable"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./static"
	}

	var db *sql.DB
	var err error
	for i := 0; i < 40; i++ {
		db, err = sql.Open("postgres", dbURL)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			break
		}
		log.Printf("等待数据库就绪 (%v)", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("无法连接数据库: %v", err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}
	if err := seed(db); err != nil {
		log.Fatalf("种子数据失败: %v", err)
	}
	// 启动时跑一次逾期归还扫描
	s := &Server{db: db, sessions: map[string]User{}}
	if n, err := s.scanLateReturns(); err == nil && n > 0 {
		log.Printf("启动扫描：发现 %d 只逾期未归还酒桶", n)
	}

	mux := http.NewServeMux()
	s.routes(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "接口不存在")
	})
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir()))))
	mux.HandleFunc("/", spaHandler(staticDir))

	log.Printf("KegTrace 已启动，监听 :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.db.Ping(); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "db not ready")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/uploads", s.auth(s.handleUpload))

	// 以下均需登录
	mux.HandleFunc("GET /api/me", s.auth(s.handleMe))
	mux.HandleFunc("GET /api/dashboard", s.auth(s.handleDashboard))
	mux.HandleFunc("GET /api/meta", s.auth(s.handleMeta))

	mux.HandleFunc("GET /api/batches", s.auth(s.handleBatchList))
	mux.HandleFunc("POST /api/batches", s.auth(s.handleBatchCreate))
	mux.HandleFunc("GET /api/batches/{id}", s.auth(s.handleBatchDetail))
	mux.HandleFunc("POST /api/batches/{id}/suspend", s.auth(s.handleBatchSuspend))
	mux.HandleFunc("POST /api/batches/{id}/recall", s.auth(s.handleBatchRecall))

	mux.HandleFunc("GET /api/investigations", s.auth(s.handleInvestigationList))
	mux.HandleFunc("GET /api/investigations/{id}", s.auth(s.handleInvestigationDetail))
	mux.HandleFunc("POST /api/investigations/{id}/conclude", s.auth(s.handleInvestigationConclude))
	mux.HandleFunc("POST /api/restock/{id}/assign", s.auth(s.handleRestockAssign))
	mux.HandleFunc("POST /api/restock/{id}/advance", s.auth(s.handleRestockAdvance))

	mux.HandleFunc("GET /api/kegs", s.auth(s.handleKegList))
	mux.HandleFunc("POST /api/kegs", s.auth(s.handleKegCreate))
	mux.HandleFunc("GET /api/kegs/{id}", s.auth(s.handleKegDetail))
	mux.HandleFunc("POST /api/kegs/{id}/open", s.auth(s.handleKegOpen))
	mux.HandleFunc("POST /api/kegs/{id}/feedback", s.auth(s.handleKegFeedback))
	mux.HandleFunc("POST /api/kegs/{id}/empty", s.auth(s.handleKegEmpty))
	mux.HandleFunc("POST /api/kegs/{id}/lost", s.auth(s.handleKegLost))
	mux.HandleFunc("POST /api/kegs/{id}/refill", s.auth(s.handleKegRefill))
	mux.HandleFunc("POST /api/kegs/{id}/wash", s.auth(s.handleKegWash))

	mux.HandleFunc("GET /api/shipments", s.auth(s.handleShipmentList))
	mux.HandleFunc("POST /api/shipments", s.auth(s.handleShipmentCreate))
	mux.HandleFunc("GET /api/shipments/{id}", s.auth(s.handleShipmentDetail))
	mux.HandleFunc("POST /api/shipments/{id}/start", s.auth(s.handleShipmentStart))
	mux.HandleFunc("POST /api/shipments/{id}/deliver", s.auth(s.handleShipmentDeliver))
	mux.HandleFunc("POST /api/shipments/{id}/receive", s.auth(s.handleShipmentReceive))
	mux.HandleFunc("POST /api/shipments/{id}/cancel", s.auth(s.handleShipmentCancel))

	mux.HandleFunc("GET /api/pickups", s.auth(s.handlePickupList))
	mux.HandleFunc("POST /api/pickups", s.auth(s.handlePickupCreate))
	mux.HandleFunc("GET /api/pickups/{id}", s.auth(s.handlePickupDetail))
	mux.HandleFunc("POST /api/pickups/{id}/items/{itemId}/scan", s.auth(s.handlePickupScan))
	mux.HandleFunc("POST /api/pickups/{id}/complete", s.auth(s.handlePickupComplete))

	mux.HandleFunc("GET /api/cases", s.auth(s.handleCaseList))
	mux.HandleFunc("GET /api/cases/{id}", s.auth(s.handleCaseDetail))
	mux.HandleFunc("POST /api/cases/{id}/notes", s.auth(s.handleCaseNote))
	mux.HandleFunc("POST /api/cases/{id}/resolve", s.auth(s.handleCaseResolve))

	mux.HandleFunc("GET /api/deposits", s.auth(s.handleDepositList))
	mux.HandleFunc("GET /api/deposits/summary", s.auth(s.handleDepositSummary))
	mux.HandleFunc("POST /api/deposits/{id}/settle", s.auth(s.handleDepositSettle))
	mux.HandleFunc("POST /api/deposits/deduct", s.auth(s.handleDepositDeduct))

	mux.HandleFunc("GET /api/notifications", s.auth(s.handleNotificationList))
	mux.HandleFunc("POST /api/notifications/read", s.auth(s.handleNotificationRead))

	mux.HandleFunc("POST /api/checks/late-returns", s.auth(s.handleLateReturnCheck))

	mux.HandleFunc("GET /api/admin/users", s.auth(s.handleAdminUsers))
	mux.HandleFunc("POST /api/admin/users", s.auth(s.handleAdminUserCreate))
	mux.HandleFunc("GET /api/admin/dealers", s.auth(s.handleAdminDealers))
	mux.HandleFunc("POST /api/admin/dealers", s.auth(s.handleAdminDealerCreate))
	mux.HandleFunc("GET /api/admin/bars", s.auth(s.handleAdminBars))
	mux.HandleFunc("POST /api/admin/bars", s.auth(s.handleAdminBarCreate))
}

// spaHandler 提供前端静态文件，未命中路径回退到 index.html（前端路由）。
func spaHandler(dir string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(dir))
	return func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() && !strings.HasSuffix(r.URL.Path, "/") {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	}
}
