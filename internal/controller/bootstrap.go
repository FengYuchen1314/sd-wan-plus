package controller

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/artifacts"
	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleBootstrapInstall(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "token required", 400)
		return
	}
	t, err := s.db.GetEnrollmentToken(token)
	if err != nil {
		http.Error(w, "invalid token: not found", 403)
		return
	}
	if t.Revoked {
		http.Error(w, "invalid token: revoked", 403)
		return
	}
	if t.UsedAt != nil {
		http.Error(w, "invalid token: already used", 403)
		return
	}
	if t.ExpiresAt.IsZero() || time.Now().UTC().After(t.ExpiresAt.UTC()) {
		http.Error(w, "invalid token: expired", 403)
		return
	}
	parent, err := s.db.GetNode(t.ParentNodeID)
	if err != nil {
		http.Error(w, "parent missing", 500)
		return
	}
	// Child must use the Host it actually reached (parent), never GitHub / hard-coded controller.
	host := r.Host
	if host == "" {
		host = fmt.Sprintf("%s:%d", s.cfg.PublicAddress, parent.NodeServicePort)
	}
	base := "http://" + host
	name := t.SuggestedNodeName
	if q := r.URL.Query().Get("name"); q != "" {
		name = q
	}
	script := artifacts.InstallScript(token, base, name, core.ProductVersion, parent.NodeServicePort)
	w.Header().Set("Content-Type", "text/x-shellscript")
	_, _ = io.WriteString(w, script)
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token             string `json:"token"`
		NodeName          string `json:"node_name"`
		WGPublicKey       string `json:"wg_public_key"`
		IdentityPublicKey string `json:"identity_public_key"`
		AgentVersion      string `json:"agent_version"`
		ProtocolVersion   int    `json:"protocol_version"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	t, err := s.db.GetEnrollmentToken(body.Token)
	if err != nil {
		writeJSON(w, 403, map[string]string{"code": string(core.ErrNotFound), "message": "token not found"})
		return
	}
	if t.Revoked {
		writeJSON(w, 403, map[string]string{"code": string(core.ErrTokenRevoked), "message": "revoked"})
		return
	}
	if t.UsedAt != nil {
		writeJSON(w, 403, map[string]string{"code": string(core.ErrTokenUsed), "message": "already used"})
		return
	}
	if t.ExpiresAt.IsZero() || time.Now().UTC().After(t.ExpiresAt.UTC()) {
		writeJSON(w, 403, map[string]string{"code": string(core.ErrTokenExpired), "message": "expired"})
		return
	}
	netw, err := s.db.GetNetwork()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	parent, err := s.db.GetNode(t.ParentNodeID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": "parent missing"})
		return
	}
	ip, err := s.db.AllocateOverlayIPv4(netw.OverlayIPv4CIDR)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	name := body.NodeName
	if name == "" {
		name = t.SuggestedNodeName
	}
	encPriv, _ := s.box.Encrypt("")
	encID, _ := s.box.Encrypt("")
	parentID := parent.ID
	node := &core.Node{
		DisplayName: name, OverlayIPv4: ip, WGPublicKey: body.WGPublicKey,
		WGPrivateKeyEncrypted: encPriv, IdentityPublicKey: body.IdentityPublicKey,
		IdentityPrivateKeyEnc: encID, ControlParentID: &parentID,
		NodeServicePort: parent.NodeServicePort, WGPortRangeStart: parent.WGPortRangeStart,
		WGPortRangeEnd: parent.WGPortRangeEnd, AgentVersion: body.AgentVersion,
		ProtocolVersion: body.ProtocolVersion, EnrollmentTokenID: &t.ID,
	}
	if node.ProtocolVersion == 0 {
		node.ProtocolVersion = core.ProtocolVersion
	}
	if err := s.db.CreateNode(node); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	_ = s.db.CreateControlRelation(parent.ID, node.ID, t.ID)

	port, err := s.db.AllocateWGPort(parent.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	addrs, _ := s.db.ListNodeAddresses(parent.ID)
	listenerAddr := s.cfg.PublicAddress
	if len(addrs) > 0 {
		listenerAddr = addrs[0].Address
	}
	link := &core.WireGuardLink{
		NodeA: parent.ID, NodeB: node.ID, InitiatorNodeID: node.ID, ListenerNodeID: parent.ID,
		ListenerAddress: listenerAddr, ListenerPort: port,
		InterfaceNameA: fmt.Sprintf("pwl-%s", node.ID[:8]),
		InterfaceNameB: fmt.Sprintf("pwl-%s", parent.ID[:8]),
		Enabled: true, AdminWeight: 1, Status: core.LinkActive,
	}
	if len(link.InterfaceNameA) > 15 {
		link.InterfaceNameA = link.InterfaceNameA[:15]
	}
	if len(link.InterfaceNameB) > 15 {
		link.InterfaceNameB = link.InterfaceNameB[:15]
	}
	_ = s.db.CreateLink(link)
	ep := fmt.Sprintf("%s:%d", listenerAddr, port)
	_ = s.db.CreateLinkEndpoint(&core.WireGuardLinkEndpoint{
		LinkID: link.ID, NodeID: parent.ID, InterfaceName: link.InterfaceNameA,
		ListenPort: port, PeerPublicKey: body.WGPublicKey, IsInitiator: false,
	})
	_ = s.db.CreateLinkEndpoint(&core.WireGuardLinkEndpoint{
		LinkID: link.ID, NodeID: node.ID, InterfaceName: link.InterfaceNameB,
		PeerEndpoint: &ep, PeerPublicKey: parent.WGPublicKey, PersistentKeepalive: 25, IsInitiator: true,
	})

	s.notify("nodes", node)
	writeJSON(w, 200, map[string]any{
		"node_id": node.ID, "overlay_ipv4": ip, "network_id": netw.ID,
		"parent_wg_public_key": parent.WGPublicKey,
		"parent_wg_endpoint":   ep,
		"link_id":              link.ID,
	})
}

func (s *Server) handleCommitEnroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID           string `json:"node_id"`
		Token            string `json:"token"`
		OverlayReachable bool   `json:"overlay_reachable"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	t, err := s.db.GetEnrollmentToken(body.Token)
	if err != nil {
		writeJSON(w, 403, map[string]string{"message": "bad token"})
		return
	}
	_ = s.db.MarkTokenUsed(t.ID)
	parent, _ := s.db.GetNode(t.ParentNodeID)
	recovery := s.cfg.PublicAddress
	if parent != nil {
		addrs, _ := s.db.ListNodeAddresses(parent.ID)
		if len(addrs) > 0 {
			recovery = fmt.Sprintf("%s:%d", addrs[0].Address, parent.NodeServicePort)
		}
	}
	_ = s.db.TouchNodeSeen(body.NodeID, core.ProductVersion, nil)
	s.notify("enrollment", map[string]any{"node_id": body.NodeID, "done": true})
	writeJSON(w, 200, map[string]any{"success": true, "recovery_endpoint": recovery, "overlay_reachable": body.OverlayReachable})
}

func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		name = filepath.Base(r.URL.Path)
	}
	if s.store != nil {
		s.store.ServeHTTP(w, r, name)
		return
	}
	candidates := []string{
		filepath.Join(s.cfg.DataDir, "artifacts", name),
		filepath.Join("bin", name),
		filepath.Join("bin", name+".exe"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
	}
	http.Error(w, "artifact not found: "+name, 404)
}

func (s *Server) handleAgentDesired(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")
	if nodeID == "" {
		nodeID = filepath.Base(r.URL.Path)
	}
	s.mu.Lock()
	st := s.desired[nodeID]
	s.mu.Unlock()
	if st == nil {
		writeJSON(w, 404, map[string]string{"message": "no desired state"})
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID           string `json:"node_id"`
		AgentVersion     string `json:"agent_version"`
		ActiveGeneration *int64 `json:"active_generation"`
	}
	if err := readJSON(r, &body); err != nil || body.NodeID == "" {
		writeJSON(w, 400, map[string]string{"message": "node_id required"})
		return
	}
	_ = s.db.TouchNodeSeen(body.NodeID, body.AgentVersion, body.ActiveGeneration)
	s.notify("heartbeat", body)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")
	s.updateMu.Lock()
	job := s.activeUpdate
	s.updateMu.Unlock()
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	st, _ := s.db.GetNode(nodeID)
	depth := 0
	if st != nil {
		depth, _ = s.db.NodeDepth(nodeID)
	}
	// During install phase, only nodes that finished prefetch get install orders;
	// leaf-first: deeper nodes install first.
	phase := job.Phase
	if phase == "install" {
		s.updateMu.Lock()
		status := job.StatusByNode[nodeID]
		s.updateMu.Unlock()
		if status != core.UpdateStaged && status != core.UpdateInstalling && status != core.UpdateCompleted {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if status == core.UpdateCompleted {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeJSON(w, 200, map[string]any{
		"job_id": job.JobID, "target_version": job.TargetVersion,
		"phase": phase, "files": job.Files, "depth": depth,
	})
}

func (s *Server) handleAgentUpdateReport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID string `json:"node_id"`
		JobID  string `json:"job_id"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	targets, _ := s.db.ListUpdateTargets(body.JobID)
	for _, t := range targets {
		if t.NodeID == body.NodeID {
			_ = s.db.UpdateTargetStatus(t.ID, body.Status, body.Error)
			break
		}
	}
	s.updateMu.Lock()
	if s.activeUpdate != nil && s.activeUpdate.JobID == body.JobID {
		if s.activeUpdate.StatusByNode == nil {
			s.activeUpdate.StatusByNode = map[string]string{}
		}
		s.activeUpdate.StatusByNode[body.NodeID] = body.Status
	}
	s.updateMu.Unlock()
	s.broadcastUpdateProgress(body.JobID)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// BootstrapController initializes DB with admin + controller node.
func BootstrapController(db *storage.DB, box *security.SecretBox, cfg Config, password, controllerName string) error {
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := db.CreateAdmin(core.DefaultUsername, hash); err != nil {
		return err
	}
	netw, err := db.CreateNetwork(controllerName, cfg.OverlayCIDR)
	if err != nil {
		return err
	}
	wgPriv, wgPub, err := security.GenerateWGKeyPair()
	if err != nil {
		return err
	}
	idPriv, idPub, err := security.GenerateIdentityKeyPair()
	if err != nil {
		return err
	}
	encWG, err := box.Encrypt(wgPriv)
	if err != nil {
		return err
	}
	encID, err := box.Encrypt(idPriv)
	if err != nil {
		return err
	}
	ip, err := db.AllocateOverlayIPv4(netw.OverlayIPv4CIDR)
	if err != nil {
		return err
	}
	node := &core.Node{
		DisplayName: controllerName, OverlayIPv4: ip, WGPublicKey: wgPub,
		WGPrivateKeyEncrypted: encWG, IdentityPublicKey: idPub, IdentityPrivateKeyEnc: encID,
		NodeServicePort: cfg.NodePort, WGPortRangeStart: cfg.WGPortStart, WGPortRangeEnd: cfg.WGPortEnd,
		IsController: true, AgentVersion: core.ProductVersion, ProtocolVersion: core.ProtocolVersion,
	}
	if err := db.CreateNode(node); err != nil {
		return err
	}
	if _, err = db.AddNodeAddress(node.ID, cfg.PublicAddress, "public", true); err != nil {
		return err
	}
	// Agent 依赖 data/node_id；不要依赖主机是否安装 sqlite3 CLI
	if cfg.DataDir != "" {
		_ = os.MkdirAll(cfg.DataDir, 0o755)
		if err := os.WriteFile(filepath.Join(cfg.DataDir, "node_id"), []byte(node.ID+"\n"), 0o600); err != nil {
			return fmt.Errorf("write node_id: %w", err)
		}
	}
	return nil
}
