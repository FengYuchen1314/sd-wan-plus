package controller

import (
	"net/http"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "version": core.ProductVersion})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.Allow(ip) {
		writeJSON(w, 429, map[string]string{"code": string(core.ErrRateLimited), "message": "too many login attempts"})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil || body.Password == "" {
		writeJSON(w, 400, map[string]string{"code": string(core.ErrValidation), "message": "password required"})
		return
	}
	admin, err := s.db.GetAdminByUsername(core.DefaultUsername)
	if err != nil || !security.VerifyPassword(admin.PasswordHash, body.Password) {
		writeJSON(w, 401, map[string]string{"code": string(core.ErrUnauthorized), "message": "invalid credentials"})
		return
	}
	token, err := security.RandomToken(32)
	if err != nil {
		writeJSON(w, 500, map[string]string{"code": string(core.ErrInternal), "message": "token error"})
		return
	}
	exp := time.Now().Add(time.Duration(s.cfg.SessionHours) * time.Hour)
	sess, err := s.db.CreateSession(admin.ID, token, ip, r.UserAgent(), exp)
	if err != nil {
		writeJSON(w, 500, map[string]string{"code": string(core.ErrInternal), "message": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "pw_session", Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.cfg.TLS, Expires: exp,
	})
	_ = s.db.AddAudit(&admin.ID, "login", "session", &sess.ID, "", ip)
	writeJSON(w, 200, map[string]any{"username": admin.Username, "expires_at": exp})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	if sess != nil {
		_ = s.db.RevokeSession(sess.ID)
	}
	http.SetCookie(w, &http.Cookie{Name: "pw_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	admin := adminFrom(r.Context())
	writeJSON(w, 200, map[string]any{"id": admin.ID, "username": admin.Username})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := readJSON(r, &body); err != nil || len(body.New) < 8 {
		writeJSON(w, 400, map[string]string{"code": string(core.ErrValidation), "message": "new password must be >= 8 chars"})
		return
	}
	admin := adminFrom(r.Context())
	if !security.VerifyPassword(admin.PasswordHash, body.Current) {
		writeJSON(w, 401, map[string]string{"code": string(core.ErrUnauthorized), "message": "current password incorrect"})
		return
	}
	hash, err := security.HashPassword(body.New)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	if err := s.db.UpdateAdminPassword(admin.ID, hash); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	_ = s.db.AddAudit(&admin.ID, "change_password", "admin", &admin.ID, "", clientIP(r))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleRevokeSessions(w http.ResponseWriter, r *http.Request) {
	admin := adminFrom(r.Context())
	sess := sessionFrom(r.Context())
	_ = s.db.RevokeOtherSessions(admin.ID, sess.ID)
	_ = s.db.AddAudit(&admin.ID, "revoke_sessions", "session", nil, "", clientIP(r))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
