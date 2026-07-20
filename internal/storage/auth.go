package storage

import (
	"database/sql"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/google/uuid"
)

func (db *DB) CreateAdmin(username, passwordHash string) (*core.Admin, error) {
	now := Now()
	a := &core.Admin{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := db.SQL.Exec(`INSERT INTO admin(id, username, password_hash, created_at, updated_at) VALUES(?,?,?,?,?)`,
		a.ID, a.Username, a.PasswordHash, a.CreatedAt.Format(time.RFC3339), a.UpdatedAt.Format(time.RFC3339))
	return a, err
}

func (db *DB) scanAdmin(row interface{ Scan(dest ...any) error }) (*core.Admin, error) {
	var a core.Admin
	var cAt, uAt string
	if err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &cAt, &uAt); err != nil {
		return nil, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
	a.UpdatedAt, _ = time.Parse(time.RFC3339, uAt)
	return &a, nil
}

func (db *DB) GetAdminByUsername(username string) (*core.Admin, error) {
	return db.scanAdmin(db.SQL.QueryRow(`SELECT id, username, password_hash, created_at, updated_at FROM admin WHERE username = ?`, username))
}

func (db *DB) GetAdminByID(id string) (*core.Admin, error) {
	return db.scanAdmin(db.SQL.QueryRow(`SELECT id, username, password_hash, created_at, updated_at FROM admin WHERE id = ?`, id))
}

func (db *DB) UpdateAdminPassword(id, hash string) error {
	_, err := db.SQL.Exec(`UPDATE admin SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, NowStr(), id)
	return err
}

func (db *DB) CreateSession(adminID, token, ip, ua string, expires time.Time) (*core.Session, error) {
	s := &core.Session{
		ID: uuid.NewString(), AdminID: adminID, Token: token,
		IPAddress: ip, UserAgent: ua, CreatedAt: Now(), ExpiresAt: expires,
	}
	_, err := db.SQL.Exec(`INSERT INTO sessions(id, admin_id, token, ip_address, user_agent, created_at, expires_at, revoked)
		VALUES(?,?,?,?,?,?,?,0)`, s.ID, s.AdminID, s.Token, s.IPAddress, s.UserAgent,
		s.CreatedAt.Format(time.RFC3339), s.ExpiresAt.Format(time.RFC3339))
	return s, err
}

func (db *DB) GetSessionByToken(token string) (*core.Session, error) {
	row := db.SQL.QueryRow(`SELECT id, admin_id, token, ip_address, user_agent, created_at, expires_at, revoked FROM sessions WHERE token = ?`, token)
	var s core.Session
	var cAt, eAt string
	var revoked int
	if err := row.Scan(&s.ID, &s.AdminID, &s.Token, &s.IPAddress, &s.UserAgent, &cAt, &eAt, &revoked); err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
	s.ExpiresAt, _ = time.Parse(time.RFC3339, eAt)
	s.Revoked = revoked == 1
	return &s, nil
}

func (db *DB) RevokeSession(id string) error {
	_, err := db.SQL.Exec(`UPDATE sessions SET revoked = 1 WHERE id = ?`, id)
	return err
}

func (db *DB) RevokeOtherSessions(adminID, keepID string) error {
	_, err := db.SQL.Exec(`UPDATE sessions SET revoked = 1 WHERE admin_id = ? AND id != ?`, adminID, keepID)
	return err
}

func (db *DB) CreateNetwork(name, cidr string) (*core.Network, error) {
	n := &core.Network{ID: uuid.NewString(), Name: name, OverlayIPv4CIDR: cidr, CreatedAt: Now()}
	_, err := db.SQL.Exec(`INSERT INTO networks(id, name, overlay_ipv4_cidr, overlay_ipv6_cidr, ipv6_enabled, created_at)
		VALUES(?,?,?,?,0,?)`, n.ID, n.Name, n.OverlayIPv4CIDR, nil, n.CreatedAt.Format(time.RFC3339))
	return n, err
}

func (db *DB) GetNetwork() (*core.Network, error) {
	row := db.SQL.QueryRow(`SELECT id, name, overlay_ipv4_cidr, overlay_ipv6_cidr, ipv6_enabled, created_at FROM networks LIMIT 1`)
	var n core.Network
	var v6 sql.NullString
	var ipv6 int
	var cAt string
	if err := row.Scan(&n.ID, &n.Name, &n.OverlayIPv4CIDR, &v6, &ipv6, &cAt); err != nil {
		return nil, err
	}
	n.OverlayIPv6CIDR = v6.String
	n.IPv6Enabled = ipv6 == 1
	n.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
	return &n, nil
}
