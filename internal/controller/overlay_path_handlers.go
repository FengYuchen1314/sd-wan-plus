package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/topology"
	"github.com/go-chi/chi/v5"
)

const overlayPathDesc = "overlay-path"

func (s *Server) handleTopologyPaths(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" || from == to {
		writeJSON(w, 400, map[string]string{"message": "from and to required and must differ"})
		return
	}
	maxHops := 8
	maxPaths := 64
	if v := r.URL.Query().Get("max_hops"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxHops = n
		}
	}
	if v := r.URL.Query().Get("max_paths"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPaths = n
		}
	}
	links, err := s.db.ListLinks()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	paths := topology.SimplePaths(links, from, to, maxHops, maxPaths)
	if paths == nil {
		paths = [][]string{}
	}
	writeJSON(w, 200, map[string]any{"paths": paths})
}

func (s *Server) handleListOverlayPaths(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	list, err := s.db.ListPolicies()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	var out []map[string]any
	for _, p := range list {
		if p.Description != overlayPathDesc || !p.Enabled {
			continue
		}
		hops, err := s.db.ListPolicyHops(p.ID)
		if err != nil || len(hops) < 2 {
			continue
		}
		ids := make([]string, len(hops))
		for i, h := range hops {
			ids[i] = h.NodeID
		}
		src, dst := ids[0], ids[len(ids)-1]
		if from != "" && to != "" {
			if !((src == from && dst == to) || (src == to && dst == from)) {
				continue
			}
		}
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "hops": ids,
			"src_node_id": src, "dst_node_id": dst,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"paths": out})
}

func (s *Server) handleCreateOverlayPath(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SrcNodeID string   `json:"src_node_id"`
		DstNodeID string   `json:"dst_node_id"`
		Hops      []string `json:"hops"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"message": "invalid json"})
		return
	}
	if len(body.Hops) < 2 {
		writeJSON(w, 400, map[string]string{"message": "hops (>=2) required"})
		return
	}
	if body.SrcNodeID == "" {
		body.SrcNodeID = body.Hops[0]
	}
	if body.DstNodeID == "" {
		body.DstNodeID = body.Hops[len(body.Hops)-1]
	}
	if body.Hops[0] != body.SrcNodeID || body.Hops[len(body.Hops)-1] != body.DstNodeID {
		writeJSON(w, 400, map[string]string{"message": "hops must start at src and end at dst"})
		return
	}

	links, err := s.db.ListLinks()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	if err := topology.ValidatePath(links, body.Hops); err != nil {
		if ae, ok := err.(*core.AppError); ok {
			writeJSON(w, 400, map[string]string{"code": string(ae.Code), "message": ae.Message})
			return
		}
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}

	srcNode, err := s.db.GetNode(body.SrcNodeID)
	if err != nil {
		writeJSON(w, 400, map[string]string{"message": "src node not found"})
		return
	}
	dstNode, err := s.db.GetNode(body.DstNodeID)
	if err != nil {
		writeJSON(w, 400, map[string]string{"message": "dst node not found"})
		return
	}
	if srcNode.OverlayIPv4 == "" || dstNode.OverlayIPv4 == "" {
		writeJSON(w, 400, map[string]string{"message": "both nodes need overlay IPv4"})
		return
	}

	// Replace any existing preferred path for this unordered pair.
	_ = s.deleteOverlayPathsBetween(body.SrcNodeID, body.DstNodeID)

	srcCIDR := ensureHostSlash32(srcNode.OverlayIPv4)
	dstCIDR := ensureHostSlash32(dstNode.OverlayIPv4)
	name := fmt.Sprintf("overlay %s → %s", srcNode.DisplayName, dstNode.DisplayName)
	p := &core.TrafficPolicy{
		Name: name, Priority: 100, Enabled: true, Description: overlayPathDesc,
	}
	if err := s.db.CreatePolicy(p); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	_ = s.db.SavePolicyMatch(&core.TrafficPolicyMatch{
		PolicyID: p.ID, SourceNodeID: &body.SrcNodeID, SourceCIDR: &srcCIDR,
		DestinationNodeID: &body.DstNodeID, DestinationCIDR: &dstCIDR,
		Protocol: core.ProtoAny,
	})
	for i, hop := range body.Hops {
		_ = s.db.SavePolicyHop(&core.TrafficPolicyPathHop{PolicyID: p.ID, HopOrder: i, NodeID: hop})
	}
	last := body.Hops[len(body.Hops)-1]
	_ = s.db.SavePolicyConfig(&core.TrafficPolicyConfig{
		PolicyID: p.ID, EgressNodeID: &last, ReturnPathType: "Symmetric",
	})

	if _, err := s.publishConfig(fmt.Sprintf("overlay path %s→%s", body.SrcNodeID, body.DstNodeID)); err != nil {
		writeJSON(w, 400, map[string]string{"message": "saved but publish failed: " + err.Error()})
		return
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "create_overlay_path", "policy", &p.ID, name, clientIP(r))
	s.notify("overlay-paths", p)
	writeJSON(w, 200, map[string]any{
		"id": p.ID, "name": p.Name, "hops": body.Hops,
		"src_node_id": body.SrcNodeID, "dst_node_id": body.DstNodeID,
	})
}

func (s *Server) handleDeleteOverlayPath(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.db.GetPolicy(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"message": "not found"})
		return
	}
	if p.Description != overlayPathDesc {
		writeJSON(w, 400, map[string]string{"message": "not an overlay path"})
		return
	}
	if err := s.db.DeletePolicy(id); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	if _, err := s.publishConfig("clear overlay path " + id); err != nil {
		writeJSON(w, 400, map[string]string{"message": "deleted but publish failed: " + err.Error()})
		return
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "delete_overlay_path", "policy", &id, p.Name, clientIP(r))
	s.notify("overlay-paths", map[string]string{"id": id})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) deleteOverlayPathsBetween(a, b string) error {
	list, err := s.db.ListPolicies()
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.Description != overlayPathDesc {
			continue
		}
		hops, err := s.db.ListPolicyHops(p.ID)
		if err != nil || len(hops) < 2 {
			continue
		}
		src, dst := hops[0].NodeID, hops[len(hops)-1].NodeID
		if (src == a && dst == b) || (src == b && dst == a) {
			_ = s.db.DeletePolicy(p.ID)
		}
	}
	return nil
}

func ensureHostSlash32(ip string) string {
	if ip == "" {
		return ""
	}
	for i := 0; i < len(ip); i++ {
		if ip[i] == '/' {
			return ip
		}
	}
	return ip + "/32"
}
