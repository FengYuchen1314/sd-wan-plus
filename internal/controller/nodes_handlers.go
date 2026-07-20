package controller

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/netutil"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
	"github.com/FengYuchen1314/sd-wan-plus/internal/topology"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.db.ListNodes()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	statuses := make([]core.NodeStatus, 0, len(nodes))
	now := time.Now()
	for _, n := range nodes {
		online := n.LastSeenAt != nil && now.Sub(*n.LastSeenAt) < 90*time.Second
		consistent := n.DesiredGeneration != nil && n.ActiveGeneration != nil && *n.DesiredGeneration == *n.ActiveGeneration
		statuses = append(statuses, core.NodeStatus{
			NodeID: n.ID, DisplayName: n.DisplayName, AgentOnline: online,
			ConfigConsistent: consistent, WGInterfacesOK: true,
			LastHandshakeAt: n.LastHandshakeAt, OverlayReachable: online,
			PathAvailable: online, UpdateConsistent: true,
			AgentVersion: n.AgentVersion, DesiredGeneration: n.DesiredGeneration, ActiveGeneration: n.ActiveGeneration,
		})
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "statuses": statuses})
}

func (s *Server) handleRenameNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := readJSON(r, &body); err != nil || body.DisplayName == "" {
		writeJSON(w, 400, map[string]string{"message": "display_name required"})
		return
	}
	if err := s.db.RenameNode(id, body.DisplayName); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "rename_node", "node", &id, body.DisplayName, clientIP(r))
	s.notify("nodes", nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		NodeServicePort  *int `json:"node_service_port"`
		WGListenPort     *int `json:"wg_listen_port"` // convenience: sets start=end
		WGPortRangeStart *int `json:"wg_port_range_start"`
		WGPortRangeEnd   *int `json:"wg_port_range_end"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	n, err := s.db.GetNode(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"message": "node not found"})
		return
	}
	sp, ws, we := n.NodeServicePort, n.WGPortRangeStart, n.WGPortRangeEnd
	if body.NodeServicePort != nil {
		if *body.NodeServicePort < 1 || *body.NodeServicePort > 65535 {
			writeJSON(w, 400, map[string]string{"message": "node_service_port out of range"})
			return
		}
		sp = *body.NodeServicePort
	}
	if body.WGListenPort != nil {
		if !validUDPPort(*body.WGListenPort) {
			writeJSON(w, 400, map[string]string{"message": "wg_listen_port out of range"})
			return
		}
		ws, we = *body.WGListenPort, *body.WGListenPort
	}
	if body.WGPortRangeStart != nil {
		if !validUDPPort(*body.WGPortRangeStart) {
			writeJSON(w, 400, map[string]string{"message": "wg_port_range_start out of range"})
			return
		}
		ws = *body.WGPortRangeStart
		if body.WGPortRangeEnd == nil && body.WGListenPort == nil && s.nodePublicAdvertise(id) == "" {
			we = ws
		}
	}
	if body.WGPortRangeEnd != nil {
		if !validUDPPort(*body.WGPortRangeEnd) {
			writeJSON(w, 400, map[string]string{"message": "wg_port_range_end out of range"})
			return
		}
		we = *body.WGPortRangeEnd
	}
	if ws > we {
		writeJSON(w, 400, map[string]string{"message": "wg port range start > end"})
		return
	}
	if err := s.db.UpdateNodePorts(id, sp, ws, we); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "update_node_ports", "node", &id, fmt.Sprintf("wg=%d-%d", ws, we), clientIP(r))
	s.notify("nodes", nil)
	writeJSON(w, 200, map[string]any{
		"status": "ok", "node_service_port": sp,
		"wg_port_range_start": ws, "wg_port_range_end": we,
	})
}

func validUDPPort(p int) bool {
	return p >= 1 && p <= 65535
}

func (s *Server) handleListAddresses(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListNodeAddresses(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleAddAddress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Address     string `json:"address"`
		AddressType string `json:"address_type"`
		IsPrimary   bool   `json:"is_primary"`
	}
	if err := readJSON(r, &body); err != nil || body.Address == "" {
		writeJSON(w, 400, map[string]string{"message": "address required"})
		return
	}
	if body.AddressType == "" {
		body.AddressType = "public"
	}
	a, err := s.db.AddNodeAddress(id, body.Address, body.AddressType, body.IsPrimary)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) handleDeleteNodeImpact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	impact, err := s.db.NodeDeleteImpact(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"message": "node not found"})
		return
	}
	writeJSON(w, 200, impact)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	impact, err := s.db.NodeDeleteImpact(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"message": "node not found"})
		return
	}
	if !impact.CanDelete {
		writeJSON(w, 409, map[string]string{"code": string(core.ErrConflict), "message": impact.BlockReason})
		return
	}
	if err := s.db.DeleteNode(id); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	s.mu.Lock()
	delete(s.desired, id)
	s.mu.Unlock()
	if _, err := s.publishConfig("auto after delete node " + impact.DisplayName); err != nil {
		log.Printf("publish after delete node: %v", err)
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "delete_node", "node", &id, impact.DisplayName, clientIP(r))
	s.notify("nodes", map[string]any{"deleted": id})
	writeJSON(w, 200, map[string]any{
		"status": "ok", "deleted": id, "impact": impact,
		"note": "节点已从控制面移除并已发布配置。请在该设备上执行卸载脚本清理本机服务。",
	})
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.db.ListLinks()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, links)
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeA           string `json:"node_a"`
		NodeB           string `json:"node_b"`
		InitiatorNodeID string `json:"initiator_node_id"`
		ListenerAddress string `json:"listener_address"`
		ListenerPort    *int   `json:"listener_port"` // optional; omit = AllocateWGPort (seamless)
		AdminWeight     int    `json:"admin_weight"`
		Enabled         bool   `json:"enabled"`
		Bidirectional   bool   `json:"bidirectional"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	if body.NodeA == "" || body.NodeB == "" || body.InitiatorNodeID == "" || body.ListenerAddress == "" {
		writeJSON(w, 400, map[string]string{"message": "node_a, node_b, initiator_node_id, listener_address required"})
		return
	}
	exists, err := s.db.LinkExistsBetween(body.NodeA, body.NodeB)
	if err != nil || exists {
		writeJSON(w, 409, map[string]string{"code": string(core.ErrConflict), "message": "link already exists"})
		return
	}
	listener := body.NodeB
	if body.InitiatorNodeID == body.NodeB {
		listener = body.NodeA
	}
	initiator := body.InitiatorNodeID
	na, _ := s.db.GetNode(body.NodeA)
	nb, _ := s.db.GetNode(body.NodeB)
	if na == nil || nb == nil {
		writeJSON(w, 404, map[string]string{"message": "node not found"})
		return
	}
	initAdv := s.nodePublicAdvertise(initiator)
	listenAdv := body.ListenerAddress
	if !netutil.IsPublicDialable(listenAdv) {
		if alt := s.nodePublicAdvertise(listener); alt != "" {
			listenAdv = alt
		}
	}
	if body.Bidirectional {
		if !netutil.IsPublicDialable(listenAdv) || initAdv == "" {
			writeJSON(w, 400, map[string]string{"message": "bidirectional requires both nodes to have public dialable addresses"})
			return
		}
	}
	var portL int
	if body.ListenerPort != nil {
		if !validUDPPort(*body.ListenerPort) {
			writeJSON(w, 400, map[string]string{"message": "listener_port out of range"})
			return
		}
		portL = *body.ListenerPort
	} else {
		var err error
		portL, err = s.db.AllocateWGPort(listener)
		if err != nil {
			writeJSON(w, 400, map[string]string{"message": err.Error()})
			return
		}
	}
	portI := 0
	if body.Bidirectional {
		portI, err = s.db.AllocateWGPort(initiator)
		if err != nil {
			writeJSON(w, 400, map[string]string{"message": err.Error()})
			return
		}
	}
	weight := body.AdminWeight
	if weight <= 0 {
		weight = 1
	}
	link := &core.WireGuardLink{
		NodeA: body.NodeA, NodeB: body.NodeB, InitiatorNodeID: body.InitiatorNodeID,
		ListenerNodeID: listener, ListenerAddress: listenAdv, ListenerPort: portL,
		InterfaceNameA: storage.InterfaceName(body.NodeA, body.NodeB),
		InterfaceNameB: storage.InterfaceName(body.NodeB, body.NodeA),
		Enabled: body.Enabled, Bidirectional: body.Bidirectional, AdminWeight: weight, Status: core.LinkPending,
	}
	if err := s.db.CreateLink(link); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	// 默认单向：initiator 拨 listener；仅手动 bidirectional 时双侧互拨。
	listenToInit := fmt.Sprintf("%s:%d", listenAdv, portL)
	epA := &core.WireGuardLinkEndpoint{
		LinkID: link.ID, NodeID: body.NodeA, InterfaceName: link.InterfaceNameA,
		PeerPublicKey: nb.WGPublicKey, IsInitiator: body.InitiatorNodeID == body.NodeA,
	}
	epB := &core.WireGuardLinkEndpoint{
		LinkID: link.ID, NodeID: body.NodeB, InterfaceName: link.InterfaceNameB,
		PeerPublicKey: na.WGPublicKey, IsInitiator: body.InitiatorNodeID == body.NodeB,
	}
	var reverse *string
	if body.Bidirectional && initAdv != "" && portI > 0 {
		ep := fmt.Sprintf("%s:%d", initAdv, portI)
		reverse = &ep
	}
	if body.InitiatorNodeID == body.NodeA {
		epA.PeerEndpoint = &listenToInit
		epA.PersistentKeepalive = 25
		epB.ListenPort = portL
		if body.Bidirectional {
			epA.ListenPort = portI
			epB.PeerEndpoint = reverse
			epB.PersistentKeepalive = 25
		}
	} else {
		epB.PeerEndpoint = &listenToInit
		epB.PersistentKeepalive = 25
		epA.ListenPort = portL
		if body.Bidirectional {
			epB.ListenPort = portI
			epA.PeerEndpoint = reverse
			epA.PersistentKeepalive = 25
		}
	}
	_ = s.db.CreateLinkEndpoint(epA)
	_ = s.db.CreateLinkEndpoint(epB)
	if body.Enabled {
		_ = s.db.SetLinkEnabled(link.ID, true)
		link.Enabled = true
		link.Status = core.LinkActive
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "create_link", "link", &link.ID, "", clientIP(r))
	if body.Enabled {
		if _, err := s.publishConfig("auto after create link"); err != nil {
			log.Printf("publish after create link: %v", err)
		}
	}
	s.notify("links", link)
	writeJSON(w, 200, link)
}

func (s *Server) handleUpdateLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Enabled       *bool `json:"enabled"`
		Bidirectional *bool `json:"bidirectional"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	changed := false
	if body.Bidirectional != nil {
		link, err := s.db.GetLink(id)
		if err != nil || link == nil {
			writeJSON(w, 404, map[string]string{"message": "link not found"})
			return
		}
		if *body.Bidirectional {
			initAdv := s.nodePublicAdvertise(link.InitiatorNodeID)
			listenAdv := link.ListenerAddress
			if !netutil.IsPublicDialable(listenAdv) {
				listenAdv = s.nodePublicAdvertise(link.ListenerNodeID)
			}
			if initAdv == "" || !netutil.IsPublicDialable(listenAdv) {
				writeJSON(w, 400, map[string]string{"message": "bidirectional requires both nodes to have public dialable addresses"})
				return
			}
		}
		if err := s.db.SetLinkBidirectional(id, *body.Bidirectional); err != nil {
			writeJSON(w, 500, map[string]string{"message": err.Error()})
			return
		}
		changed = true
	}
	if body.Enabled != nil {
		if err := s.db.SetLinkEnabled(id, *body.Enabled); err != nil {
			writeJSON(w, 500, map[string]string{"message": err.Error()})
			return
		}
		changed = true
	}
	if changed {
		if _, err := s.publishConfig("auto after update link"); err != nil {
			log.Printf("publish after update link: %v", err)
		}
	}
	link, _ := s.db.GetLink(id)
	writeJSON(w, 200, link)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeleteLink(id); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "delete_link", "link", &id, "", clientIP(r))
	s.notify("links", nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	g, err := topology.Build(s.db)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, g)
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListEnrollmentTokens()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ParentNodeID      string `json:"parent_node_id"`
		SuggestedNodeName string `json:"suggested_node_name"`
		ParentAddress     string `json:"parent_address"`
		ExpiresMinutes    int    `json:"expires_minutes"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	parent, err := s.db.GetNode(body.ParentNodeID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"message": "parent not found"})
		return
	}
	netw, err := s.db.GetNetwork()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	if body.ExpiresMinutes <= 0 {
		body.ExpiresMinutes = 60
	}
	tok, err := security.RandomToken(24)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	t := &core.EnrollmentToken{
		Token: tok, NetworkID: netw.ID, ParentNodeID: parent.ID,
		SuggestedNodeName: body.SuggestedNodeName, AllowedInstallMode: "node",
		ExpiresAt: time.Now().UTC().Add(time.Duration(body.ExpiresMinutes) * time.Minute),
	}
	if err := s.db.CreateEnrollmentToken(t); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	addr := body.ParentAddress
	if addr == "" {
		addrs, _ := s.db.ListNodeAddresses(parent.ID)
		if len(addrs) > 0 {
			addr = addrs[0].Address
		} else {
			addr = s.cfg.PublicAddress
		}
	}
	// 先下载再执行，避免 curl|bash 时交互提示被进度条盖住、看起来像卡住
	cmd := fmt.Sprintf(`tmp=$(mktemp) && curl -fsSL "http://%s:%d/bootstrap/install.sh?token=%s" -o "$tmp" && sudo bash "$tmp"; rm -f "$tmp"`, addr, parent.NodeServicePort, tok)
	unified := fmt.Sprintf("sudo bash install.sh --role node --parent-url http://%s:%d --token %s --name %s", addr, parent.NodeServicePort, tok, body.SuggestedNodeName)
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "create_enrollment_token", "enrollment_token", &t.ID, body.SuggestedNodeName, clientIP(r))
	writeJSON(w, 200, map[string]any{
		"token": t, "install_command": cmd, "install_command_unified": unified,
		"parent_address": addr, "parent_port": parent.NodeServicePort,
		"note": "链式安装：全部文件只从父节点拉取；主控与子节点使用同一 Release 安装包",
	})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.RevokeToken(id); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
