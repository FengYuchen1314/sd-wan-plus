package storage

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/google/uuid"
)

func (db *DB) CreateEnrollmentToken(t *core.EnrollmentToken) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	t.CreatedAt = Now()
	rev := 0
	if t.Revoked {
		rev = 1
	}
	_, err := db.SQL.Exec(`INSERT INTO enrollment_tokens(id, token, network_id, parent_node_id, suggested_node_name, allowed_install_mode, expires_at, used_at, revoked, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Token, t.NetworkID, t.ParentNodeID, t.SuggestedNodeName, t.AllowedInstallMode,
		t.ExpiresAt.UTC().Format(time.RFC3339), NullTime(t.UsedAt), rev, t.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

func (db *DB) GetEnrollmentToken(token string) (*core.EnrollmentToken, error) {
	row := db.SQL.QueryRow(`SELECT id, token, network_id, parent_node_id, suggested_node_name, allowed_install_mode, expires_at, used_at, revoked, created_at
		FROM enrollment_tokens WHERE token = ?`, token)
	var t core.EnrollmentToken
	var exp, used, cAt sql.NullString
	var rev int
	if err := row.Scan(&t.ID, &t.Token, &t.NetworkID, &t.ParentNodeID, &t.SuggestedNodeName, &t.AllowedInstallMode, &exp, &used, &rev, &cAt); err != nil {
		return nil, err
	}
	if exp.Valid {
		t.ExpiresAt = parseFlexibleTime(exp.String)
	}
	t.UsedAt = ParseTime(used)
	t.Revoked = rev == 1
	if cAt.Valid {
		t.CreatedAt = parseFlexibleTime(cAt.String)
	}
	return &t, nil
}

func parseFlexibleTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func (db *DB) ListEnrollmentTokens() ([]core.EnrollmentToken, error) {
	rows, err := db.SQL.Query(`SELECT id, token, network_id, parent_node_id, suggested_node_name, allowed_install_mode, expires_at, used_at, revoked, created_at
		FROM enrollment_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.EnrollmentToken
	for rows.Next() {
		var t core.EnrollmentToken
		var exp, cAt string
		var rev int
		var usedNS sql.NullString
		if err := rows.Scan(&t.ID, &t.Token, &t.NetworkID, &t.ParentNodeID, &t.SuggestedNodeName, &t.AllowedInstallMode, &exp, &usedNS, &rev, &cAt); err != nil {
			return nil, err
		}
		t.ExpiresAt, _ = time.Parse(time.RFC3339, exp)
		t.UsedAt = ParseTime(usedNS)
		t.Revoked = rev == 1
		t.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
		out = append(out, t)
	}
	return out, nil
}

func (db *DB) MarkTokenUsed(id string) error {
	_, err := db.SQL.Exec(`UPDATE enrollment_tokens SET used_at = ? WHERE id = ?`, NowStr(), id)
	return err
}

func (db *DB) RevokeToken(id string) error {
	_, err := db.SQL.Exec(`UPDATE enrollment_tokens SET revoked = 1 WHERE id = ?`, id)
	return err
}

func (db *DB) CreatePolicy(p *core.TrafficPolicy) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	en := 1
	if !p.Enabled {
		en = 0
	}
	_, err := db.SQL.Exec(`INSERT INTO traffic_policies(id, name, priority, enabled, description, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Priority, en, p.Description, p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339))
	return err
}

func (db *DB) ListPolicies() ([]core.TrafficPolicy, error) {
	rows, err := db.SQL.Query(`SELECT id, name, priority, enabled, description, created_at, updated_at FROM traffic_policies ORDER BY priority`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.TrafficPolicy
	for rows.Next() {
		var p core.TrafficPolicy
		var en int
		var desc sql.NullString
		var cAt, uAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.Priority, &en, &desc, &cAt, &uAt); err != nil {
			return nil, err
		}
		p.Enabled = en == 1
		p.Description = desc.String
		p.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, uAt)
		out = append(out, p)
	}
	return out, nil
}

func (db *DB) GetPolicy(id string) (*core.TrafficPolicy, error) {
	row := db.SQL.QueryRow(`SELECT id, name, priority, enabled, description, created_at, updated_at FROM traffic_policies WHERE id = ?`, id)
	var p core.TrafficPolicy
	var en int
	var desc sql.NullString
	var cAt, uAt string
	if err := row.Scan(&p.ID, &p.Name, &p.Priority, &en, &desc, &cAt, &uAt); err != nil {
		return nil, err
	}
	p.Enabled = en == 1
	p.Description = desc.String
	p.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, uAt)
	return &p, nil
}

func (db *DB) DeletePolicy(id string) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM traffic_policy_matches WHERE policy_id = ?`,
		`DELETE FROM traffic_policy_paths WHERE policy_id = ?`,
		`DELETE FROM traffic_policy_configs WHERE policy_id = ?`,
		`DELETE FROM traffic_policies WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) SavePolicyMatch(m *core.TrafficPolicyMatch) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	_, err := db.SQL.Exec(`INSERT INTO traffic_policy_matches(id, policy_id, source_node_id, source_node_group, source_cidr, destination_node_id, destination_cidr, protocol, destination_port_start, destination_port_end)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.PolicyID, NullStr(m.SourceNodeID), NullStr(m.SourceNodeGroup), NullStr(m.SourceCIDR),
		NullStr(m.DestinationNodeID), NullStr(m.DestinationCIDR), m.Protocol,
		nullInt(m.DestinationPortStart), nullInt(m.DestinationPortEnd))
	return err
}

func nullInt(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}

func (db *DB) SavePolicyHop(h *core.TrafficPolicyPathHop) error {
	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	_, err := db.SQL.Exec(`INSERT INTO traffic_policy_paths(id, policy_id, hop_order, node_id) VALUES(?,?,?,?)`,
		h.ID, h.PolicyID, h.HopOrder, h.NodeID)
	return err
}

func (db *DB) SavePolicyConfig(c *core.TrafficPolicyConfig) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	nat := 0
	if c.EgressNAT {
		nat = 1
	}
	fo := 0
	if c.FailoverEnabled {
		fo = 1
	}
	_, err := db.SQL.Exec(`INSERT INTO traffic_policy_configs(id, policy_id, egress_node_id, egress_nat, return_path_type, failover_enabled, failover_path_id)
		VALUES(?,?,?,?,?,?,?)`,
		c.ID, c.PolicyID, NullStr(c.EgressNodeID), nat, c.ReturnPathType, fo, NullStr(c.FailoverPathID))
	return err
}

func (db *DB) ListPolicyHops(policyID string) ([]core.TrafficPolicyPathHop, error) {
	rows, err := db.SQL.Query(`SELECT id, policy_id, hop_order, node_id FROM traffic_policy_paths WHERE policy_id = ? ORDER BY hop_order`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.TrafficPolicyPathHop
	for rows.Next() {
		var h core.TrafficPolicyPathHop
		if err := rows.Scan(&h.ID, &h.PolicyID, &h.HopOrder, &h.NodeID); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

func (db *DB) ListPolicyMatches(policyID string) ([]core.TrafficPolicyMatch, error) {
	rows, err := db.SQL.Query(`SELECT id, policy_id, source_node_id, source_node_group, source_cidr, destination_node_id, destination_cidr, protocol, destination_port_start, destination_port_end
		FROM traffic_policy_matches WHERE policy_id = ?`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.TrafficPolicyMatch
	for rows.Next() {
		var m core.TrafficPolicyMatch
		var sn, sg, sc, dn, dc sql.NullString
		var ps, pe sql.NullInt64
		if err := rows.Scan(&m.ID, &m.PolicyID, &sn, &sg, &sc, &dn, &dc, &m.Protocol, &ps, &pe); err != nil {
			return nil, err
		}
		m.SourceNodeID = StrPtr(sn)
		m.SourceNodeGroup = StrPtr(sg)
		m.SourceCIDR = StrPtr(sc)
		m.DestinationNodeID = StrPtr(dn)
		m.DestinationCIDR = StrPtr(dc)
		if ps.Valid {
			v := int(ps.Int64)
			m.DestinationPortStart = &v
		}
		if pe.Valid {
			v := int(pe.Int64)
			m.DestinationPortEnd = &v
		}
		out = append(out, m)
	}
	return out, nil
}

func (db *DB) GetPolicyConfig(policyID string) (*core.TrafficPolicyConfig, error) {
	row := db.SQL.QueryRow(`SELECT id, policy_id, egress_node_id, egress_nat, return_path_type, failover_enabled, failover_path_id FROM traffic_policy_configs WHERE policy_id = ?`, policyID)
	var c core.TrafficPolicyConfig
	var eg, fo sql.NullString
	var nat, fail int
	if err := row.Scan(&c.ID, &c.PolicyID, &eg, &nat, &c.ReturnPathType, &fail, &fo); err != nil {
		return nil, err
	}
	c.EgressNodeID = StrPtr(eg)
	c.EgressNAT = nat == 1
	c.FailoverEnabled = fail == 1
	c.FailoverPathID = StrPtr(fo)
	return &c, nil
}

func (db *DB) CreateConfigRevision(reason string, generation int64) (*core.ConfigRevision, error) {
	r := &core.ConfigRevision{ID: uuid.NewString(), Generation: generation, Reason: reason, Status: "Pending", CreatedAt: Now()}
	_, err := db.SQL.Exec(`INSERT INTO config_revisions(id, generation, reason, status, created_at) VALUES(?,?,?,?,?)`,
		r.ID, r.Generation, r.Reason, r.Status, r.CreatedAt.Format(time.RFC3339))
	return r, err
}

func (db *DB) NextGeneration() (int64, error) {
	var max sql.NullInt64
	err := db.SQL.QueryRow(`SELECT MAX(generation) FROM config_revisions`).Scan(&max)
	if err != nil {
		return 1, err
	}
	if !max.Valid {
		return 1, nil
	}
	return max.Int64 + 1, nil
}

func (db *DB) SaveDesiredConfig(revID, nodeID string, state *core.NodeDesiredState) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = db.SQL.Exec(`INSERT INTO node_desired_configs(id, config_revision_id, node_id, config_data, config_hash) VALUES(?,?,?,?,?)`,
		uuid.NewString(), revID, nodeID, string(b), state.ConfigHash)
	return err
}

func (db *DB) CreateRolloutNode(revID, nodeID string) error {
	_, err := db.SQL.Exec(`INSERT INTO config_rollout_nodes(id, config_revision_id, node_id, status) VALUES(?,?,?,?)`,
		uuid.NewString(), revID, nodeID, core.RolloutPending)
	return err
}

func (db *DB) ListConfigRevisions() ([]core.ConfigRevision, error) {
	rows, err := db.SQL.Query(`SELECT id, generation, reason, status, created_at, completed_at FROM config_revisions ORDER BY generation DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.ConfigRevision
	for rows.Next() {
		var r core.ConfigRevision
		var cAt string
		var done sql.NullString
		if err := rows.Scan(&r.ID, &r.Generation, &r.Reason, &r.Status, &cAt, &done); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
		r.CompletedAt = ParseTime(done)
		out = append(out, r)
	}
	return out, nil
}

func (db *DB) UpdateRolloutStatus(revID, nodeID, status, errMsg string) error {
	_, err := db.SQL.Exec(`UPDATE config_rollout_nodes SET status=?, error_message=?, completed_at=CASE WHEN ? IN ('Active','RolledBack','PrepareFailed','ActivateFailed','VerifyFailed') THEN ? ELSE completed_at END WHERE config_revision_id=? AND node_id=?`,
		status, errMsg, status, NowStr(), revID, nodeID)
	return err
}

func (db *DB) ListRolloutNodes(revID string) ([]map[string]any, error) {
	rows, err := db.SQL.Query(`SELECT id, node_id, status, started_at, completed_at, error_message FROM config_rollout_nodes WHERE config_revision_id = ?`, revID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, nodeID, status string
		var started, completed, errmsg sql.NullString
		if err := rows.Scan(&id, &nodeID, &status, &started, &completed, &errmsg); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "node_id": nodeID, "status": status,
			"started_at": started.String, "completed_at": completed.String, "error_message": errmsg.String,
		})
	}
	return out, nil
}

func (db *DB) AddAudit(adminID *string, action, resType string, resID *string, details, ip string) error {
	_, err := db.SQL.Exec(`INSERT INTO audit_logs(id, admin_id, action, resource_type, resource_id, details, ip_address, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		uuid.NewString(), NullStr(adminID), action, resType, NullStr(resID), details, ip, NowStr())
	return err
}

func (db *DB) ListAuditLogs(limit int) ([]core.AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.SQL.Query(`SELECT id, admin_id, action, resource_type, resource_id, details, ip_address, created_at FROM audit_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.AuditLog
	for rows.Next() {
		var a core.AuditLog
		var admin, res, details sql.NullString
		var cAt string
		if err := rows.Scan(&a.ID, &admin, &a.Action, &a.ResourceType, &res, &details, &a.IPAddress, &cAt); err != nil {
			return nil, err
		}
		a.AdminID = StrPtr(admin)
		a.ResourceID = StrPtr(res)
		a.Details = details.String
		a.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
		out = append(out, a)
	}
	return out, nil
}

func (db *DB) CreateUpdateJob(version, sha string) (*core.UpdateJob, error) {
	j := &core.UpdateJob{ID: uuid.NewString(), TargetVersion: version, ManifestSHA256: sha, Status: "Created", CreatedAt: Now()}
	_, err := db.SQL.Exec(`INSERT INTO update_jobs(id, target_version, manifest_sha256, status, created_at) VALUES(?,?,?,?,?)`,
		j.ID, j.TargetVersion, j.ManifestSHA256, j.Status, j.CreatedAt.Format(time.RFC3339))
	return j, err
}

func (db *DB) ListUpdateJobs() ([]core.UpdateJob, error) {
	rows, err := db.SQL.Query(`SELECT id, target_version, manifest_sha256, status, created_at, completed_at FROM update_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.UpdateJob
	for rows.Next() {
		var j core.UpdateJob
		var cAt string
		var done sql.NullString
		if err := rows.Scan(&j.ID, &j.TargetVersion, &j.ManifestSHA256, &j.Status, &cAt, &done); err != nil {
			return nil, err
		}
		j.CreatedAt, _ = time.Parse(time.RFC3339, cAt)
		j.CompletedAt = ParseTime(done)
		out = append(out, j)
	}
	return out, nil
}

func (db *DB) AddUpdateTarget(jobID, nodeID string, depth int) error {
	_, err := db.SQL.Exec(`INSERT INTO update_targets(id, update_job_id, node_id, depth, status) VALUES(?,?,?,?,?)`,
		uuid.NewString(), jobID, nodeID, depth, core.UpdateWaiting)
	return err
}

func (db *DB) ListUpdateTargets(jobID string) ([]core.UpdateTarget, error) {
	rows, err := db.SQL.Query(`SELECT id, update_job_id, node_id, depth, status, started_at, completed_at, error_message FROM update_targets WHERE update_job_id = ? ORDER BY depth DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.UpdateTarget
	for rows.Next() {
		var t core.UpdateTarget
		var started, completed, errmsg sql.NullString
		if err := rows.Scan(&t.ID, &t.UpdateJobID, &t.NodeID, &t.Depth, &t.Status, &started, &completed, &errmsg); err != nil {
			return nil, err
		}
		t.StartedAt = ParseTime(started)
		t.CompletedAt = ParseTime(completed)
		t.ErrorMessage = errmsg.String
		out = append(out, t)
	}
	return out, nil
}

func (db *DB) UpdateJobStatus(id, status string) error {
	done := ""
	if status == "Completed" || status == "RolledBack" {
		done = NowStr()
	}
	if done != "" {
		_, err := db.SQL.Exec(`UPDATE update_jobs SET status=?, completed_at=? WHERE id=?`, status, done, id)
		return err
	}
	_, err := db.SQL.Exec(`UPDATE update_jobs SET status=? WHERE id=?`, status, id)
	return err
}

func (db *DB) UpdateTargetStatus(id, status, errMsg string) error {
	_, err := db.SQL.Exec(`UPDATE update_targets SET status=?, error_message=?, started_at=COALESCE(started_at, ?), completed_at=CASE WHEN ? IN ('Completed','RolledBack','InstallFailed','HealthCheckFailed','DownloadFailed','SignatureInvalid') THEN ? ELSE completed_at END WHERE id=?`,
		status, errMsg, NowStr(), status, NowStr(), id)
	return err
}
