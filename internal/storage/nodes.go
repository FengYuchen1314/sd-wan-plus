package storage

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/google/uuid"
)

func scanNode(row interface{ Scan(dest ...any) error }) (*core.Node, error) {
	var n core.Node
	var parent, enroll sql.NullString
	var desired, active sql.NullInt64
	var lastSeen, lastHS sql.NullString
	var isCtrl int
	var cAt, uAt string
	var agent sql.NullString
	err := row.Scan(
		&n.ID, &n.DisplayName, &n.OverlayIPv4, &n.OverlayIPv6,
		&n.WGPublicKey, &n.WGPrivateKeyEncrypted, &n.IdentityPublicKey, &n.IdentityPrivateKeyEnc,
		&parent, &n.NodeServicePort, &n.WGPortRangeStart, &n.WGPortRangeEnd, &isCtrl,
		&agent, &n.ProtocolVersion, &desired, &active, &cAt, &uAt, &lastSeen, &lastHS, &enroll,
	)
	if err != nil {
		return nil, err
	}
	n.ControlParentID = StrPtr(parent)
	n.IsController = isCtrl == 1
	n.AgentVersion = agent.String
	n.DesiredGeneration = Int64Ptr(desired)
	n.ActiveGeneration = Int64Ptr(active)
	n.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, uAt)
	n.LastSeenAt = ParseTime(lastSeen)
	n.LastHandshakeAt = ParseTime(lastHS)
	n.EnrollmentTokenID = StrPtr(enroll)
	return &n, nil
}

const nodeCols = `id, display_name, overlay_ipv4, overlay_ipv6, wg_public_key, wg_private_key_encrypted,
	identity_public_key, identity_private_key_encrypted, control_parent_id, node_service_port,
	wg_port_range_start, wg_port_range_end, is_controller, agent_version, protocol_version,
	desired_generation, active_generation, created_at, updated_at, last_seen_at, last_handshake_at, enrollment_token_id`

func (db *DB) CreateNode(n *core.Node) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	now := Now()
	n.CreatedAt = now
	n.UpdatedAt = now
	isCtrl := 0
	if n.IsController {
		isCtrl = 1
	}
	_, err := db.SQL.Exec(`INSERT INTO nodes(`+nodeCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		n.ID, n.DisplayName, n.OverlayIPv4, n.OverlayIPv6, n.WGPublicKey, n.WGPrivateKeyEncrypted,
		n.IdentityPublicKey, n.IdentityPrivateKeyEnc, NullStr(n.ControlParentID), n.NodeServicePort,
		n.WGPortRangeStart, n.WGPortRangeEnd, isCtrl, n.AgentVersion, n.ProtocolVersion,
		NullInt64(n.DesiredGeneration), NullInt64(n.ActiveGeneration),
		n.CreatedAt.Format(time.RFC3339), n.UpdatedAt.Format(time.RFC3339),
		NullTime(n.LastSeenAt), NullTime(n.LastHandshakeAt), NullStr(n.EnrollmentTokenID),
	)
	return err
}

func (db *DB) GetNode(id string) (*core.Node, error) {
	return scanNode(db.SQL.QueryRow(`SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id))
}

func (db *DB) ListNodes() ([]core.Node, error) {
	rows, err := db.SQL.Query(`SELECT ` + nodeCols + ` FROM nodes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (db *DB) RenameNode(id, name string) error {
	_, err := db.SQL.Exec(`UPDATE nodes SET display_name = ?, updated_at = ? WHERE id = ?`, name, NowStr(), id)
	return err
}

func (db *DB) UpdateNodePorts(id string, servicePort, wgStart, wgEnd int) error {
	_, err := db.SQL.Exec(`UPDATE nodes SET node_service_port=?, wg_port_range_start=?, wg_port_range_end=?, updated_at=? WHERE id=?`,
		servicePort, wgStart, wgEnd, NowStr(), id)
	return err
}

func (db *DB) TouchNodeSeen(id string, agentVersion string, activeGen *int64) error {
	_, err := db.SQL.Exec(`UPDATE nodes SET last_seen_at=?, agent_version=?, active_generation=?, updated_at=? WHERE id=?`,
		NowStr(), agentVersion, NullInt64(activeGen), NowStr(), id)
	return err
}

func (db *DB) GetControllerNode() (*core.Node, error) {
	return scanNode(db.SQL.QueryRow(`SELECT ` + nodeCols + ` FROM nodes WHERE is_controller = 1 LIMIT 1`))
}

func (db *DB) AllocateOverlayIPv4(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	nodes, err := db.ListNodes()
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, n := range nodes {
		used[n.OverlayIPv4] = true
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("not ipv4 cidr")
	}
	// start from .1
	for i := 1; i < 65534; i++ {
		cand := make(net.IP, 4)
		copy(cand, ip)
		cand[2] = byte(i >> 8)
		cand[3] = byte(i)
		if !ipNet.Contains(cand) {
			break
		}
		s := cand.String()
		if !used[s] {
			return s, nil
		}
	}
	return "", fmt.Errorf("overlay pool exhausted")
}

func (db *DB) AddNodeAddress(nodeID, address, addrType string, primary bool) (*core.NodeAddress, error) {
	a := &core.NodeAddress{ID: uuid.NewString(), NodeID: nodeID, Address: address, AddressType: addrType, IsPrimary: primary, CreatedAt: Now()}
	p := 0
	if primary {
		p = 1
	}
	_, err := db.SQL.Exec(`INSERT INTO node_addresses(id, node_id, address, address_type, is_primary, created_at) VALUES(?,?,?,?,?,?)`,
		a.ID, a.NodeID, a.Address, a.AddressType, p, a.CreatedAt.Format(time.RFC3339))
	return a, err
}

func (db *DB) ListNodeAddresses(nodeID string) ([]core.NodeAddress, error) {
	rows, err := db.SQL.Query(`SELECT id, node_id, address, address_type, is_primary, created_at FROM node_addresses WHERE node_id = ? ORDER BY is_primary DESC, CASE address_type WHEN 'public' THEN 0 WHEN 'lan' THEN 1 ELSE 2 END, created_at ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.NodeAddress
	for rows.Next() {
		var a core.NodeAddress
		var p int
		var cAt string
		if err := rows.Scan(&a.ID, &a.NodeID, &a.Address, &a.AddressType, &p, &cAt); err != nil {
			return nil, err
		}
		a.IsPrimary = p == 1
		a.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
		out = append(out, a)
	}
	return out, nil
}

func (db *DB) CreateControlRelation(parentID, childID, tokenID string) error {
	_, err := db.SQL.Exec(`INSERT INTO control_relations(id, parent_id, child_id, enrolled_at, enrollment_token_id) VALUES(?,?,?,?,?)`,
		uuid.NewString(), parentID, childID, NowStr(), tokenID)
	return err
}

func (db *DB) ListControlRelations() ([]core.ControlRelation, error) {
	rows, err := db.SQL.Query(`SELECT id, parent_id, child_id, enrolled_at, enrollment_token_id FROM control_relations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.ControlRelation
	for rows.Next() {
		var r core.ControlRelation
		var eAt string
		if err := rows.Scan(&r.ID, &r.ParentID, &r.ChildID, &eAt, &r.EnrollmentTokenID); err != nil {
			return nil, err
		}
		r.EnrolledAt, _ = time.Parse(time.RFC3339, eAt)
		out = append(out, r)
	}
	return out, nil
}

func (db *DB) NodeDepth(nodeID string) (int, error) {
	depth := 0
	id := nodeID
	for {
		n, err := db.GetNode(id)
		if err != nil {
			return 0, err
		}
		if n.ControlParentID == nil {
			return depth, nil
		}
		depth++
		id = *n.ControlParentID
		if depth > 1000 {
			return 0, fmt.Errorf("control tree cycle detected")
		}
	}
}

func (db *DB) ChildrenOf(parentID string) ([]core.Node, error) {
	rows, err := db.SQL.Query(`SELECT `+nodeCols+` FROM nodes WHERE control_parent_id = ?`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}

// NodeDeleteImpact summarizes what deleting a node would affect (panel confirm UX).
type NodeDeleteImpact struct {
	NodeID       string   `json:"node_id"`
	DisplayName  string   `json:"display_name"`
	IsController bool     `json:"is_controller"`
	CanDelete    bool     `json:"can_delete"`
	BlockReason  string   `json:"block_reason,omitempty"`
	ChildCount   int      `json:"child_count"`
	ChildNames   []string `json:"child_names,omitempty"`
	LinkCount    int      `json:"link_count"`
	LinkIDs      []string `json:"link_ids,omitempty"`
	Warning      string   `json:"warning,omitempty"`
}

func (db *DB) NodeDeleteImpact(id string) (*NodeDeleteImpact, error) {
	n, err := db.GetNode(id)
	if err != nil {
		return nil, err
	}
	out := &NodeDeleteImpact{
		NodeID: n.ID, DisplayName: n.DisplayName, IsController: n.IsController,
		CanDelete: true,
	}
	if n.IsController {
		out.CanDelete = false
		out.BlockReason = "控制机不可删除。如需重建，请整机卸载后重装控制机。"
		return out, nil
	}
	children, err := db.ChildrenOf(id)
	if err != nil {
		return nil, err
	}
	out.ChildCount = len(children)
	for _, c := range children {
		out.ChildNames = append(out.ChildNames, c.DisplayName)
	}
	if out.ChildCount > 0 {
		out.CanDelete = false
		out.BlockReason = fmt.Sprintf("仍有 %d 个控制树下级，请先删除或迁移下级节点。", out.ChildCount)
	}
	links, err := db.ListLinks()
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if l.NodeA == id || l.NodeB == id {
			out.LinkCount++
			out.LinkIDs = append(out.LinkIDs, l.ID)
		}
	}
	out.Warning = fmt.Sprintf("将删除节点「%s」及 %d 条相邻 WireGuard 链路，并自动发布新配置。请随后在该设备上执行卸载。",
		n.DisplayName, out.LinkCount)
	return out, nil
}

func execIgnoreMissing(tx *sql.Tx, query string, args ...any) error {
	_, err := tx.Exec(query, args...)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

func (db *DB) DeleteNode(id string) error {
	impact, err := db.NodeDeleteImpact(id)
	if err != nil {
		return err
	}
	if !impact.CanDelete {
		return fmt.Errorf("%s", impact.BlockReason)
	}
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	rollback := func(e error) error { _ = tx.Rollback(); return e }

	rows, err := tx.Query(`SELECT id FROM wireguard_links WHERE node_a=? OR node_b=?`, id, id)
	if err != nil {
		return rollback(err)
	}
	var linkIDs []string
	for rows.Next() {
		var lid string
		if err := rows.Scan(&lid); err != nil {
			rows.Close()
			return rollback(err)
		}
		linkIDs = append(linkIDs, lid)
	}
	rows.Close()
	for _, lid := range linkIDs {
		if err := execIgnoreMissing(tx, `DELETE FROM wireguard_link_endpoints WHERE link_id=?`, lid); err != nil {
			return rollback(err)
		}
		if _, err := tx.Exec(`DELETE FROM wireguard_links WHERE id=?`, lid); err != nil {
			return rollback(err)
		}
	}

	if err := execIgnoreMissing(tx, `DELETE FROM control_relations WHERE parent_id=? OR child_id=?`, id, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM node_addresses WHERE node_id=?`, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM enrollment_tokens WHERE parent_node_id=?`, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM update_targets WHERE node_id=?`, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM config_rollout_nodes WHERE node_id=?`, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM node_desired_configs WHERE node_id=?`, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM heartbeats WHERE node_id=?`, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM node_port_pools WHERE node_id=?`, id); err != nil {
		return rollback(err)
	}
	if err := execIgnoreMissing(tx, `DELETE FROM traffic_policy_paths WHERE node_id=?`, id); err != nil {
		return rollback(err)
	}
	_ = execIgnoreMissing(tx, `UPDATE traffic_policy_matches SET source_node_id=NULL WHERE source_node_id=?`, id)
	_ = execIgnoreMissing(tx, `UPDATE traffic_policy_matches SET destination_node_id=NULL WHERE destination_node_id=?`, id)
	_ = execIgnoreMissing(tx, `UPDATE traffic_policy_configs SET egress_node_id=NULL WHERE egress_node_id=?`, id)
	_ = execIgnoreMissing(tx, `UPDATE nodes SET control_parent_id=NULL WHERE control_parent_id=?`, id)

	if _, err := tx.Exec(`DELETE FROM nodes WHERE id=?`, id); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

