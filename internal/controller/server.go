package controller

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/artifacts"
	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/ports"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
)

type Config struct {
	WebPort        int
	NodePort       int
	WGPortStart    int
	WGPortEnd      int
	PublicAddress  string
	OverlayCIDR    string
	DBPath         string
	StaticDir      string
	KeyFile        string
	SessionHours   int
	MaxLoginAttempts int
	LoginWindowSec int
	TLS            bool
	DataDir        string
}

func DefaultConfig() Config {
	return Config{
		WebPort: ports.Web, NodePort: ports.Node, WGPortStart: ports.WGStart, WGPortEnd: ports.WGEnd,
		PublicAddress: "127.0.0.1", OverlayCIDR: "10.250.0.0/16",
		DBPath: "", StaticDir: "", KeyFile: "",
		SessionHours: 48, MaxLoginAttempts: 5, LoginWindowSec: 300, DataDir: "./data",
	}
}

type Server struct {
	cfg      Config
	db       *storage.DB
	box      *security.SecretBox
	limiter  *security.RateLimiter
	hub      *WSHub
	mu       sync.Mutex
	desired  map[string]*core.NodeDesiredState
	store    *artifacts.Store
	updateMu sync.Mutex
	activeUpdate *activeUpdateJob

	ghMu       sync.Mutex
	ghCached   *latestReleaseInfo
	ghCachedAt time.Time
	ghCachedErr string
}

type activeUpdateJob struct {
	JobID         string
	TargetVersion string
	Phase         string // prefetch | install
	Files         []string
	StatusByNode  map[string]string
}

func New(cfg Config) (*Server, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "pathweaver.db")
	}
	if cfg.KeyFile == "" {
		cfg.KeyFile = filepath.Join(cfg.DataDir, ".pathweaver.key")
	}
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	box, err := security.LoadOrCreateKey(cfg.KeyFile)
	if err != nil {
		db.Close()
		return nil, err
	}
	artDir := filepath.Join(cfg.DataDir, "artifacts")
	_ = os.MkdirAll(artDir, 0o755)
	store := artifacts.New(artDir, "")
	for _, dir := range []string{"bin", filepath.Join("..", "bin"), "/opt/pathweaver/bin"} {
		_ = store.Seed(dir, artifacts.NodeBinaries)
	}
	// Cache unified installer as install-node.sh for chain installs
	for _, cand := range []string{"install.sh", "/opt/pathweaver/install.sh", filepath.Join(cfg.DataDir, "..", "install.sh")} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			_ = copyFileSimple(cand, filepath.Join(artDir, "install-node.sh"))
			break
		}
	}

	s := &Server{
		cfg: cfg, db: db, box: box,
		limiter: security.NewRateLimiter(cfg.MaxLoginAttempts, time.Duration(cfg.LoginWindowSec)*time.Second),
		hub:     NewWSHub(),
		desired: map[string]*core.NodeDesiredState{},
		store:   store,
	}
	return s, nil
}

func (s *Server) Close() error { return s.db.Close() }

func (s *Server) DB() *storage.DB { return s.db }
func (s *Server) Box() *security.SecretBox { return s.box }
func (s *Server) Cfg() Config { return s.cfg }

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/api/health", s.handleHealth)

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", s.handleLogin)
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
			r.Post("/change-password", s.handleChangePassword)
			r.Post("/revoke-sessions", s.handleRevokeSessions)
		})
	})

	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)
		r.Get("/api/nodes", s.handleListNodes)
		r.Put("/api/nodes/{id}", s.handleUpdateNode)
		r.Put("/api/nodes/{id}/rename", s.handleRenameNode)
		r.Get("/api/nodes/{id}/delete-impact", s.handleDeleteNodeImpact)
		r.Delete("/api/nodes/{id}", s.handleDeleteNode)
		r.Get("/api/nodes/{id}/addresses", s.handleListAddresses)
		r.Post("/api/nodes/{id}/addresses", s.handleAddAddress)

		r.Get("/api/links", s.handleListLinks)
		r.Post("/api/links", s.handleCreateLink)
		r.Put("/api/links/{id}", s.handleUpdateLink)
		r.Delete("/api/links/{id}", s.handleDeleteLink)

		r.Get("/api/policies", s.handleListPolicies)
		r.Post("/api/policies", s.handleCreatePolicy)
		r.Get("/api/policies/{id}", s.handleGetPolicy)
		r.Delete("/api/policies/{id}", s.handleDeletePolicy)

		r.Get("/api/enrollment/tokens", s.handleListTokens)
		r.Post("/api/enrollment/tokens", s.handleCreateToken)
		r.Post("/api/enrollment/tokens/{id}/revoke", s.handleRevokeToken)

		r.Get("/api/topology", s.handleTopology)
		r.Get("/api/config/preview", s.handleConfigPreview)
		r.Post("/api/config/publish", s.handleConfigPublish)
		r.Get("/api/config/revisions", s.handleListRevisions)
		r.Get("/api/config/revisions/{id}/nodes", s.handleRevisionNodes)

		r.Get("/api/updates", s.handleListUpdates)
		r.Get("/api/updates/latest", s.handleUpdatesLatest)
		r.Get("/api/updates/overview", s.handleUpdatesOverview)
		r.Post("/api/updates/pull-latest", s.handlePullLatest)
		r.Post("/api/updates", s.handleCreateUpdate)
		r.Post("/api/updates/{id}/start", s.handleStartUpdate)
		r.Post("/api/updates/{id}/rollback", s.handleRollbackUpdate)
		r.Get("/api/updates/{id}/targets", s.handleUpdateTargets)

		r.Get("/api/audit-logs", s.handleAuditLogs)
		r.Get("/api/dashboard", s.handleDashboard)
	})

	r.Get("/ws", s.handleWS)

	// Bootstrap / node enrollment (no admin session; token-based)
	r.Get("/bootstrap/install.sh", s.handleBootstrapInstall)
	r.Post("/bootstrap/enroll", s.handleEnroll)
	r.Post("/bootstrap/commit", s.handleCommitEnroll)
	r.Get("/bootstrap/artifact/{name}", s.handleArtifact)

	// Agent pull desired config (node auth via node id header for V1; mTLS later)
	r.Get("/api/agent/desired/{nodeID}", s.handleAgentDesired)
	r.Post("/api/agent/heartbeat", s.handleAgentHeartbeat)
	r.Get("/api/agent/update/{nodeID}", s.handleAgentUpdate)
	r.Post("/api/agent/update/report", s.handleAgentUpdateReport)

	if s.cfg.StaticDir != "" {
		fileServer(r, s.cfg.StaticDir)
	}
	return r
}

func (s *Server) NodeRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Get("/bootstrap/install.sh", s.handleBootstrapInstall)
	r.Post("/bootstrap/enroll", s.handleEnroll)
	r.Post("/bootstrap/commit", s.handleCommitEnroll)
	r.Get("/bootstrap/artifact/{name}", s.handleArtifact)
	r.Get("/api/agent/desired/{nodeID}", s.handleAgentDesired)
	r.Post("/api/agent/heartbeat", s.handleAgentHeartbeat)
	r.Get("/api/agent/update/{nodeID}", s.handleAgentUpdate)
	r.Post("/api/agent/update/report", s.handleAgentUpdateReport)
	r.Get("/api/health", s.handleHealth)
	return r
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	go s.hub.Run()
	s.hydrateDesiredFromDB()
	go s.resumeOrFinalizeUpdates()
	webAddr := formatAddr(s.cfg.WebPort)
	nodeAddr := formatAddr(s.cfg.NodePort)
	webSrv := &http.Server{Addr: webAddr, Handler: s.Router()}
	nodeSrv := &http.Server{Addr: nodeAddr, Handler: s.NodeRouter()}
	go func() {
		<-ctx.Done()
		_ = webSrv.Shutdown(context.Background())
		_ = nodeSrv.Shutdown(context.Background())
	}()
	errCh := make(chan error, 2)
	go func() {
		log.Printf("PathWeaver web listening on %s", webAddr)
		errCh <- webSrv.ListenAndServe()
	}()
	go func() {
		log.Printf("PathWeaver node service listening on %s", nodeAddr)
		errCh <- nodeSrv.ListenAndServe()
	}()
	err := <-errCh
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func formatAddr(port int) string {
	return ":" + itoa(port)
}

func copyFileSimple(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		if w.Header().Get("Access-Control-Allow-Origin") == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func fileServer(r chi.Router, dir string) {
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); err != nil {
		log.Printf("WARNING: static UI missing at %s (%v) — web console will not load", index, err)
	}
	fs := http.FileServer(http.Dir(dir))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		rel := strings.TrimPrefix(filepath.Clean(req.URL.Path), "/")
		if rel == "." || rel == "" {
			http.ServeFile(w, req, index)
			return
		}
		full := filepath.Join(dir, rel)
		// prevent escaping static root
		if !strings.HasPrefix(full, filepath.Clean(dir)+string(os.PathSeparator)) && full != filepath.Clean(dir) {
			http.Error(w, "forbidden", 403)
			return
		}
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, req)
			return
		}
		// SPA fallback
		http.ServeFile(w, req, index)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Real-Ip"); xff != "" {
		return xff
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

type ctxKey int

const ctxAdmin ctxKey = 1
const ctxSession ctxKey = 2

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("pw_session")
		if err != nil || c.Value == "" {
			writeJSON(w, 401, map[string]string{"code": string(core.ErrUnauthorized), "message": "not logged in"})
			return
		}
		sess, err := s.db.GetSessionByToken(c.Value)
		if err != nil || sess.Revoked || time.Now().After(sess.ExpiresAt) {
			writeJSON(w, 401, map[string]string{"code": string(core.ErrUnauthorized), "message": "session expired"})
			return
		}
		admin, err := s.db.GetAdminByID(sess.AdminID)
		if err != nil {
			writeJSON(w, 401, map[string]string{"code": string(core.ErrUnauthorized), "message": "invalid session"})
			return
		}
		ctx := context.WithValue(r.Context(), ctxAdmin, admin)
		ctx = context.WithValue(ctx, ctxSession, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func adminFrom(ctx context.Context) *core.Admin {
	a, _ := ctx.Value(ctxAdmin).(*core.Admin)
	return a
}

func sessionFrom(ctx context.Context) *core.Session {
	s, _ := ctx.Value(ctxSession).(*core.Session)
	return s
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

type WSHub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
	broadcast chan any
}

func NewWSHub() *WSHub {
	return &WSHub{clients: map[*websocket.Conn]bool{}, broadcast: make(chan any, 64)}
}

func (h *WSHub) Run() {
	for msg := range h.broadcast {
		h.mu.Lock()
		for c := range h.clients {
			if err := c.WriteJSON(msg); err != nil {
				_ = c.Close()
				delete(h.clients, c)
			}
		}
		h.mu.Unlock()
	}
}

func (h *WSHub) Broadcast(msg any) {
	select {
	case h.broadcast <- msg:
	default:
	}
}

func (h *WSHub) Add(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

func (h *WSHub) Remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	// optional auth via cookie
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.hub.Add(c)
	defer func() {
		s.hub.Remove(c)
		_ = c.Close()
	}()
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}

func (s *Server) notify(event string, data any) {
	s.hub.Broadcast(map[string]any{"event": event, "data": data, "ts": time.Now().UTC()})
}
