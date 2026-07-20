package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/google/uuid"
)

func (db *DB) CreateLink(l *core.WireGuardLink) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.CreatedAt = Now()
	en := 0
	if l.Enabled {
		en = 1
	}
	_, err := db.SQL.Exec(`INSERT INTO wireguard_links(
		id, node_a, node_b, initiator_node_id, listener_node_id, listener_address, listener_port,
		interface_name_a, interface_name_b, enabled, admin_weight, last_handshake_a, last_handshake_b, status, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.ID, l.NodeA, l.NodeB, l.InitiatorNodeID, l.ListenerNodeID, l.ListenerAddress, l.ListenerPort,
		l.InterfaceNameA, l.InterfaceNameB, en, l.AdminWeight, NullTime(l.LastHandshakeA), NullTime(l.LastHandshakeB),
		l.Status, l.CreatedAt.Format(time.RFC3339))
	return err
}

func (db *DB) CreateLinkEndpoint(e *core.WireGuardLinkEndpoint) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	ini := 0
	if e.IsInitiator {
		ini = 1
	}
	_, err := db.SQL.Exec(`INSERT INTO wireguard_link_endpoints(
		id, link_id, node_id, interface_name, listen_port, peer_endpoint, peer_public_key, persistent_keepalive, is_initiator)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		e.ID, e.LinkID, e.NodeID, e.InterfaceName, e.ListenPort, NullStr(e.PeerEndpoint), e.PeerPublicKey, e.PersistentKeepalive, ini)
	return err
}

func scanLink(row interface{ Scan(dest ...any) error }) (*core.WireGuardLink, error) {
	var l core.WireGuardLink
	var en int
	var hsA, hsB sql.NullString
	var cAt string
	err := row.Scan(&l.ID, &l.NodeA, &l.NodeB, &l.InitiatorNodeID, &l.ListenerNodeID, &l.ListenerAddress, &l.ListenerPort,
		&l.InterfaceNameA, &l.InterfaceNameB, &en, &l.AdminWeight, &hsA, &hsB, &l.Status, &cAt)
	if err != nil {
		return nil, err
	}
	l.Enabled = en == 1
	l.LastHandshakeA = ParseTime(hsA)
	l.LastHandshakeB = ParseTime(hsB)
	l.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
	return &l, nil
}

func (db *DB) ListLinks() ([]core.WireGuardLink, error) {
	rows, err := db.SQL.Query(`SELECT id, node_a, node_b, initiator_node_id, listener_node_id, listener_address, listener_port,
		interface_name_a, interface_name_b, enabled, admin_weight, last_handshake_a, last_handshake_b, status, created_at
		FROM wireguard_links ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.WireGuardLink
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func (db *DB) GetLink(id string) (*core.WireGuardLink, error) {
	return scanLink(db.SQL.QueryRow(`SELECT id, node_a, node_b, initiator_node_id, listener_node_id, listener_address, listener_port,
		interface_name_a, interface_name_b, enabled, admin_weight, last_handshake_a, last_handshake_b, status, created_at
		FROM wireguard_links WHERE id = ?`, id))
}

func (db *DB) DeleteLink(id string) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wireguard_link_endpoints WHERE link_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM wireguard_links WHERE id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (db *DB) SetLinkEnabled(id string, enabled bool) error {
	en := 0
	status := core.LinkDisabled
	if enabled {
		en = 1
		status = core.LinkActive
	}
	_, err := db.SQL.Exec(`UPDATE wireguard_links SET enabled = ?, status = ? WHERE id = ?`, en, status, id)
	return err
}

func (db *DB) LinkExistsBetween(a, b string) (bool, error) {
	var n int
	err := db.SQL.QueryRow(`SELECT COUNT(1) FROM wireguard_links WHERE (node_a=? AND node_b=?) OR (node_a=? AND node_b=?)`, a, b, b, a).Scan(&n)
	return n > 0, err
}

func (db *DB) AllocateWGPort(nodeID string) (int, error) {
	n, err := db.GetNode(nodeID)
	if err != nil {
		return 0, err
	}
	used := map[int]bool{}
	rows, err := db.SQL.Query(`SELECT listener_port FROM wireguard_links WHERE listener_node_id = ?`, nodeID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, err
		}
		used[p] = true
	}
	rows.Close()
	rows2, err := db.SQL.Query(`SELECT listen_port FROM wireguard_link_endpoints WHERE node_id = ? AND listen_port > 0`, nodeID)
	if err != nil {
		return 0, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var p int
		if err := rows2.Scan(&p); err != nil {
			return 0, err
		}
		used[p] = true
	}
	for p := n.WGPortRangeStart; p <= n.WGPortRangeEnd; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free WireGuard port in pool %d-%d", n.WGPortRangeStart, n.WGPortRangeEnd)
}

func (db *DB) UpdateLinkEndpoint(e *core.WireGuardLinkEndpoint) error {
	ini := 0
	if e.IsInitiator {
		ini = 1
	}
	_, err := db.SQL.Exec(`UPDATE wireguard_link_endpoints SET interface_name=?, listen_port=?, peer_endpoint=?, peer_public_key=?, persistent_keepalive=?, is_initiator=? WHERE id=?`,
		e.InterfaceName, e.ListenPort, NullStr(e.PeerEndpoint), e.PeerPublicKey, e.PersistentKeepalive, ini, e.ID)
	return err
}

func InterfaceName(a, b string) string {
	// short deterministic iface name from uuids
	short := func(id string) string {
		if len(id) >= 4 {
			return id[:4]
		}
		return id
	}
	name := "pwl-" + short(a) + short(b)
	if len(name) > 15 {
		name = name[:15]
	}
	return name
}

func (db *DB) ListLinkEndpoints(linkID string) ([]core.WireGuardLinkEndpoint, error) {
	rows, err := db.SQL.Query(`SELECT id, link_id, node_id, interface_name, listen_port, peer_endpoint, peer_public_key, persistent_keepalive, is_initiator
		FROM wireguard_link_endpoints WHERE link_id = ?`, linkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.WireGuardLinkEndpoint
	for rows.Next() {
		var e core.WireGuardLinkEndpoint
		var pe sql.NullString
		var ini int
		if err := rows.Scan(&e.ID, &e.LinkID, &e.NodeID, &e.InterfaceName, &e.ListenPort, &pe, &e.PeerPublicKey, &e.PersistentKeepalive, &ini); err != nil {
			return nil, err
		}
		e.PeerEndpoint = StrPtr(pe)
		e.IsInitiator = ini == 1
		out = append(out, e)
	}
	return out, nil
}
