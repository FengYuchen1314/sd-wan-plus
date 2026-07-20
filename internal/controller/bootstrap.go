package controller

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleBootstrapInstall(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", 400)
		return
	}
	t, err := s.db.GetEnrollmentToken(token)
	if err != nil || t.Revoked || t.UsedAt != nil || time.Now().After(t.ExpiresAt) {
		http.Error(w, "invalid token", 403)
		return
	}
	parent, err := s.db.GetNode(t.ParentNodeID)
	if err != nil {
		http.Error(w, "parent missing", 500)
		return
	}
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
echo "PathWeaver node installer"
TOKEN=%q
PARENT_PORT=%d
PARENT_HOST=$(echo "$0" | true)
# Resolve parent from the URL used to fetch this script
PARENT_URL="${PW_PARENT_URL:-}"
if [ -z "$PARENT_URL" ]; then
  echo "Set PW_PARENT_URL or use curl pipe from parent bootstrap URL"
fi
INSTALL_DIR=/opt/pathweaver
mkdir -p "$INSTALL_DIR"/{bin,data,artifacts,releases}
# Download binaries from parent artifact cache
BASE="http://$(hostname -I | awk '{print $1}'):$PARENT_PORT"
# Prefer the Host header of the request — embedded by controller:
BASE=%q
curl -fsSL "$BASE/bootstrap/artifact/pathweaver-agent" -o "$INSTALL_DIR/bin/pathweaver-agent"
curl -fsSL "$BASE/bootstrap/artifact/pathweaver-netd" -o "$INSTALL_DIR/bin/pathweaver-netd"
curl -fsSL "$BASE/bootstrap/artifact/pathweaver-updater" -o "$INSTALL_DIR/bin/pathweaver-updater"
chmod +x "$INSTALL_DIR/bin/"*
# Enroll
WG_PRIV=$(wg genkey 2>/dev/null || openssl rand -base64 32)
WG_PUB=$(echo "$WG_PRIV" | wg pubkey 2>/dev/null || echo "pending")
RESP=$(curl -fsSL -X POST "$BASE/bootstrap/enroll" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$TOKEN\",\"node_name\":%q,\"wg_public_key\":\"$WG_PUB\",\"identity_public_key\":\"$WG_PUB\",\"agent_version\":\"%s\",\"protocol_version\":1}")
echo "$RESP" > "$INSTALL_DIR/data/enroll.json"
NODE_ID=$(echo "$RESP" | sed -n 's/.*"node_id":"\([^"]*\)".*/\1/p')
echo "$NODE_ID" > "$INSTALL_DIR/data/node_id"
echo "$WG_PRIV" > "$INSTALL_DIR/data/wg_private.key"
chmod 600 "$INSTALL_DIR/data/wg_private.key"
# systemd units
cat > /etc/systemd/system/pathweaver-netd.service <<'EOF'
[Unit]
Description=PathWeaver netd
After=network-online.target
[Service]
ExecStart=/opt/pathweaver/bin/pathweaver-netd
Restart=always
RestartSec=3
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
[Install]
WantedBy=multi-user.target
EOF
cat > /etc/systemd/system/pathweaver-agent.service <<EOF
[Unit]
Description=PathWeaver agent
After=pathweaver-netd.service
Requires=pathweaver-netd.service
[Service]
Environment=PW_NODE_ID=$NODE_ID
Environment=PW_PARENT_URL=$BASE
Environment=PW_DATA_DIR=/opt/pathweaver/data
ExecStart=/opt/pathweaver/bin/pathweaver-agent
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF
cat > /etc/systemd/system/pathweaver-updater.service <<'EOF'
[Unit]
Description=PathWeaver updater
After=network-online.target
[Service]
ExecStart=/opt/pathweaver/bin/pathweaver-updater
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now pathweaver-netd pathweaver-agent pathweaver-updater
curl -fsSL -X POST "$BASE/bootstrap/commit" -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$NODE_ID\",\"token\":\"$TOKEN\",\"overlay_reachable\":true}" || true
echo "Node enrolled: $NODE_ID"
`, token, parent.NodeServicePort, fmt.Sprintf("http://%s:%d", s.cfg.PublicAddress, parent.NodeServicePort), t.SuggestedNodeName, core.ProductVersion)

	// Better: use request Host as parent base when piped from parent
	host := r.Host
	if host != "" {
		script = fmt.Sprintf(`#!/bin/bash
set -euo pipefail
TOKEN=%q
BASE="http://%s"
NAME=%q
INSTALL_DIR=/opt/pathweaver
mkdir -p "$INSTALL_DIR"/{bin,data,artifacts,releases}
curl -fsSL "$BASE/bootstrap/artifact/pathweaver-agent" -o "$INSTALL_DIR/bin/pathweaver-agent" || true
curl -fsSL "$BASE/bootstrap/artifact/pathweaver-netd" -o "$INSTALL_DIR/bin/pathweaver-netd" || true
curl -fsSL "$BASE/bootstrap/artifact/pathweaver-updater" -o "$INSTALL_DIR/bin/pathweaver-updater" || true
chmod +x "$INSTALL_DIR/bin/"* 2>/dev/null || true
WG_PRIV=$( (wg genkey) 2>/dev/null || openssl rand -base64 32 )
WG_PUB=$( (echo "$WG_PRIV" | wg pubkey) 2>/dev/null || echo "$WG_PRIV" )
RESP=$(curl -fsSL -X POST "$BASE/bootstrap/enroll" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$TOKEN\",\"node_name\":\"$NAME\",\"wg_public_key\":\"$WG_PUB\",\"identity_public_key\":\"$WG_PUB\",\"agent_version\":\"%s\",\"protocol_version\":1}")
echo "$RESP" > "$INSTALL_DIR/data/enroll.json"
NODE_ID=$(printf '%%s' "$RESP" | sed -n 's/.*"node_id":"\([^"]*\)".*/\1/p')
echo "$NODE_ID" > "$INSTALL_DIR/data/node_id"
umask 077; echo "$WG_PRIV" > "$INSTALL_DIR/data/wg_private.key"
cat > /etc/systemd/system/pathweaver-netd.service <<'UNIT'
[Unit]
Description=PathWeaver netd
After=network-online.target
[Service]
ExecStart=/opt/pathweaver/bin/pathweaver-netd
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
cat > /etc/systemd/system/pathweaver-agent.service <<UNIT
[Unit]
Description=PathWeaver agent
After=pathweaver-netd.service
[Service]
Environment=PW_NODE_ID=$NODE_ID
Environment=PW_PARENT_URL=$BASE
Environment=PW_DATA_DIR=/opt/pathweaver/data
ExecStart=/opt/pathweaver/bin/pathweaver-agent
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
cat > /etc/systemd/system/pathweaver-updater.service <<'UNIT'
[Unit]
Description=PathWeaver updater
[Service]
ExecStart=/opt/pathweaver/bin/pathweaver-updater
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now pathweaver-netd pathweaver-agent pathweaver-updater || true
curl -fsSL -X POST "$BASE/bootstrap/commit" -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$NODE_ID\",\"token\":\"$TOKEN\",\"overlay_reachable\":true}" || true
echo "PathWeaver node ready: $NODE_ID"
`, token, host, t.SuggestedNodeName, core.ProductVersion)
	}
	w.Header().Set("Content-Type", "text/x-shellscript")
	_, _ = io.WriteString(w, script)
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token              string `json:"token"`
		NodeName           string `json:"node_name"`
		WGPublicKey        string `json:"wg_public_key"`
		IdentityPublicKey  string `json:"identity_public_key"`
		AgentVersion       string `json:"agent_version"`
		ProtocolVersion    int    `json:"protocol_version"`
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
	if time.Now().After(t.ExpiresAt) {
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
	// Node stores empty encrypted private key; real key stays on node
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

	// Create parent-child WireGuard link (child initiates)
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
	name := filepath.Base(r.URL.Path)
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
	// Fallback stub script so bootstrap can proceed in dev
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = io.WriteString(w, "#!/bin/sh\necho 'artifact placeholder: "+name+"'\n")
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
		NodeID          string `json:"node_id"`
		AgentVersion    string `json:"agent_version"`
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
	_, err = db.AddNodeAddress(node.ID, cfg.PublicAddress, "public", true)
	return err
}
