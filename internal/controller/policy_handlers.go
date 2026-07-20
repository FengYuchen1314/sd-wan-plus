package controller

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/artifacts"
	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/netutil"
	"github.com/FengYuchen1314/sd-wan-plus/internal/routing"
	"github.com/FengYuchen1314/sd-wan-plus/internal/topology"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListPolicies()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.db.GetPolicy(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"message": "not found"})
		return
	}
	hops, _ := s.db.ListPolicyHops(id)
	matches, _ := s.db.ListPolicyMatches(id)
	cfg, _ := s.db.GetPolicyConfig(id)
	writeJSON(w, 200, map[string]any{"policy": p, "hops": hops, "matches": matches, "config": cfg})
}

func (s *Server) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string   `json:"name"`
		Priority    int      `json:"priority"`
		Description string   `json:"description"`
		Enabled     *bool    `json:"enabled"`
		Hops        []string `json:"hops"`
		Match       struct {
			SourceNodeID         *string `json:"source_node_id"`
			SourceCIDR           *string `json:"source_cidr"`
			DestinationNodeID    *string `json:"destination_node_id"`
			DestinationCIDR      *string `json:"destination_cidr"`
			Protocol             string  `json:"protocol"`
			DestinationPortStart *int    `json:"destination_port_start"`
			DestinationPortEnd   *int    `json:"destination_port_end"`
		} `json:"match"`
		EgressNAT      bool    `json:"egress_nat"`
		ReturnPathType string  `json:"return_path_type"`
		EgressNodeID   *string `json:"egress_node_id"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" || len(body.Hops) < 2 {
		writeJSON(w, 400, map[string]string{"message": "name and hops (>=2) required"})
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
	proto := body.Match.Protocol
	if proto == "" {
		proto = core.ProtoAny
	}
	ret := body.ReturnPathType
	if ret == "" {
		ret = "Symmetric"
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	p := &core.TrafficPolicy{Name: body.Name, Priority: body.Priority, Enabled: enabled, Description: body.Description}

	if err := s.db.CreatePolicy(p); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	_ = s.db.SavePolicyMatch(&core.TrafficPolicyMatch{
		PolicyID: p.ID, SourceNodeID: body.Match.SourceNodeID, SourceCIDR: body.Match.SourceCIDR,
		DestinationNodeID: body.Match.DestinationNodeID, DestinationCIDR: body.Match.DestinationCIDR,
		Protocol: proto, DestinationPortStart: body.Match.DestinationPortStart, DestinationPortEnd: body.Match.DestinationPortEnd,
	})
	for i, hop := range body.Hops {
		_ = s.db.SavePolicyHop(&core.TrafficPolicyPathHop{PolicyID: p.ID, HopOrder: i, NodeID: hop})
	}
	egress := body.EgressNodeID
	if egress == nil && len(body.Hops) > 0 {
		last := body.Hops[len(body.Hops)-1]
		egress = &last
	}
	_ = s.db.SavePolicyConfig(&core.TrafficPolicyConfig{
		PolicyID: p.ID, EgressNodeID: egress, EgressNAT: body.EgressNAT, ReturnPathType: ret,
	})
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "create_policy", "policy", &p.ID, p.Name, clientIP(r))
	s.notify("policies", p)
	writeJSON(w, 200, p)
}

func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeletePolicy(id); err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleConfigPreview(w http.ResponseWriter, r *http.Request) {
	gen, err := s.db.NextGeneration()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	states, err := routing.Compile(s.db, uint64(gen), s.box)
	if err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	// strip private keys from preview
	safe := map[string]any{}
	for id, st := range states {
		cp := *st
		for i := range cp.WireGuardLinks {
			cp.WireGuardLinks[i].NodePrivateKey = "***"
		}
		safe[id] = cp
	}
	writeJSON(w, 200, map[string]any{"generation": gen, "nodes": safe})
}

func (s *Server) handleConfigPublish(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	_ = readJSON(r, &body)
	if body.Reason == "" {
		body.Reason = "manual publish"
	}
	rev, err := s.publishConfig(body.Reason)
	if err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "publish_config", "config_revision", &rev.ID, body.Reason, clientIP(r))
	s.notify("config", rev)
	writeJSON(w, 200, map[string]any{"revision": rev, "node_count": len(s.desiredSnapshot())})
}

func (s *Server) desiredSnapshot() map[string]*core.NodeDesiredState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]*core.NodeDesiredState, len(s.desired))
	for k, v := range s.desired {
		out[k] = v
	}
	return out
}

// publishConfig compiles topology into desired state and makes it available to agents.
func (s *Server) publishConfig(reason string) (*core.ConfigRevision, error) {
	s.repairLinkEndpoints()
	gen, err := s.db.NextGeneration()
	if err != nil {
		return nil, err
	}
	states, err := routing.Compile(s.db, uint64(gen), s.box)
	if err != nil {
		return nil, err
	}
	rev, err := s.db.CreateConfigRevision(reason, gen)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	for nodeID, st := range states {
		_ = s.db.SaveDesiredConfig(rev.ID, nodeID, st)
		_ = s.db.CreateRolloutNode(rev.ID, nodeID)
		s.desired[nodeID] = st
		_ = s.db.UpdateRolloutStatus(rev.ID, nodeID, core.RolloutDispatched, "")
	}
	s.mu.Unlock()
	return rev, nil
}

func (s *Server) nodePublicAdvertise(nodeID string) string {
	addrs, _ := s.db.ListNodeAddresses(nodeID)
	for _, a := range addrs {
		if a.AddressType == "lan" {
			continue
		}
		if netutil.IsPublicDialable(a.Address) {
			return a.Address
		}
	}
	return ""
}

// repairLinkEndpoints normalizes endpoints: default one-way; bidirectional only when flag set.
func (s *Server) repairLinkEndpoints() {
	links, err := s.db.ListLinks()
	if err != nil {
		return
	}
	for _, l := range links {
		eps, err := s.db.ListLinkEndpoints(l.ID)
		if err != nil || len(eps) == 0 {
			continue
		}
		var initEp, listenEp *core.WireGuardLinkEndpoint
		for i := range eps {
			if eps[i].IsInitiator {
				initEp = &eps[i]
			} else {
				listenEp = &eps[i]
			}
		}
		if initEp == nil || listenEp == nil {
			continue
		}
		changed := false
		wantInitEP := fmt.Sprintf("%s:%d", l.ListenerAddress, l.ListenerPort)
		if initEp.PeerEndpoint == nil || *initEp.PeerEndpoint != wantInitEP {
			ep := wantInitEP
			initEp.PeerEndpoint = &ep
			changed = true
		}
		if initEp.PersistentKeepalive != 25 {
			initEp.PersistentKeepalive = 25
			changed = true
		}
		if listenEp.ListenPort != l.ListenerPort && l.ListenerPort > 0 {
			listenEp.ListenPort = l.ListenerPort
			changed = true
		}

		if !l.Bidirectional {
			if initEp.ListenPort != 0 {
				initEp.ListenPort = 0
				changed = true
			}
			if listenEp.PeerEndpoint != nil {
				listenEp.PeerEndpoint = nil
				changed = true
			}
			if listenEp.PersistentKeepalive != 0 {
				listenEp.PersistentKeepalive = 0
				changed = true
			}
		} else {
			initAdv := s.nodePublicAdvertise(initEp.NodeID)
			if initAdv == "" || !netutil.IsPublicDialable(l.ListenerAddress) {
				// Cannot sustain bidirectional — fall back to one-way shape.
				if initEp.ListenPort != 0 {
					initEp.ListenPort = 0
					changed = true
				}
				if listenEp.PeerEndpoint != nil {
					listenEp.PeerEndpoint = nil
					changed = true
				}
				if listenEp.PersistentKeepalive != 0 {
					listenEp.PersistentKeepalive = 0
					changed = true
				}
			} else {
				if initEp.ListenPort <= 0 {
					if p, err := s.db.AllocateWGPort(initEp.NodeID); err == nil {
						initEp.ListenPort = p
						changed = true
					}
				}
				wantRev := fmt.Sprintf("%s:%d", initAdv, initEp.ListenPort)
				if listenEp.PeerEndpoint == nil || *listenEp.PeerEndpoint != wantRev {
					ep := wantRev
					listenEp.PeerEndpoint = &ep
					changed = true
				}
				if listenEp.PersistentKeepalive != 25 {
					listenEp.PersistentKeepalive = 25
					changed = true
				}
			}
		}
		if changed {
			_ = s.db.UpdateLinkEndpoint(initEp)
			_ = s.db.UpdateLinkEndpoint(listenEp)
			mode := "one-way"
			if l.Bidirectional {
				mode = "bidirectional"
			}
			log.Printf("repaired link endpoints for %s (%s)", l.ID, mode)
		}
	}
}

// hydrateDesiredFromDB rebuilds in-memory desired state after controller restart.
func (s *Server) hydrateDesiredFromDB() {
	states, err := routing.Compile(s.db, 1, s.box)
	if err != nil {
		log.Printf("hydrate desired state: %v", err)
		return
	}
	s.mu.Lock()
	for id, st := range states {
		s.desired[id] = st
	}
	s.mu.Unlock()
	log.Printf("hydrated desired state for %d nodes", len(states))
}

func (s *Server) handleListRevisions(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListConfigRevisions()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleRevisionNodes(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListRolloutNodes(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleListUpdates(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListUpdateJobs()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetVersion  string   `json:"target_version"`
		ManifestSHA256 string   `json:"manifest_sha256"`
		Files          []string `json:"files"`
		SourceDir      string   `json:"source_dir"` // optional: stage release from this dir into artifact cache
	}
	if err := readJSON(r, &body); err != nil || body.TargetVersion == "" {
		writeJSON(w, 400, map[string]string{"message": "target_version required"})
		return
	}
	if body.ManifestSHA256 == "" {
		body.ManifestSHA256 = "pending"
	}
	files := body.Files
	if len(files) == 0 {
		files = artifacts.NodeBinaries
	}
	// Stage release into controller artifact cache (origin). Children pull only from parents.
	if body.SourceDir != "" && s.store != nil {
		if err := s.store.SeedRelease(body.TargetVersion, body.SourceDir); err != nil {
			writeJSON(w, 400, map[string]string{"message": "seed release: " + err.Error()})
			return
		}
	} else if s.store != nil {
		_ = s.store.Seed("bin", files)
		_ = s.store.Seed(filepath.Join(s.cfg.DataDir, "artifacts"), files)
		_ = s.store.SeedRelease(body.TargetVersion, "bin")
	}
	job, err := s.db.CreateUpdateJob(body.TargetVersion, body.ManifestSHA256)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	nodes, _ := s.db.ListNodes()
	for _, n := range nodes {
		depth, _ := s.db.NodeDepth(n.ID)
		_ = s.db.AddUpdateTarget(job.ID, n.ID, depth)
	}
	admin := adminFrom(r.Context())
	_ = s.db.AddAudit(&admin.ID, "create_update", "update_job", &job.ID, body.TargetVersion, clientIP(r))
	writeJSON(w, 200, map[string]any{"job": job, "files": files, "note": "artifacts staged on controller; start to prefetch via control tree"})
}

func (s *Server) handleStartUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	jobs, err := s.db.ListUpdateJobs()
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	var job *core.UpdateJob
	for i := range jobs {
		if jobs[i].ID == id {
			job = &jobs[i]
			break
		}
	}
	if job == nil {
		writeJSON(w, 404, map[string]string{"message": "job not found"})
		return
	}
	_ = s.db.UpdateJobStatus(id, "Running")
	targets, err := s.db.ListUpdateTargets(id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	go s.runUpdateJob(id, job.TargetVersion, targets)
	writeJSON(w, 200, map[string]string{"status": "started", "mode": "prefetch-then-leaf-install"})
}

func (s *Server) runUpdateJob(jobID, version string, targets []core.UpdateTarget) {
	files := artifacts.NodeBinaries
	statusByNode := map[string]string{}
	for _, t := range targets {
		if t.Status != "" && t.Status != core.UpdateWaiting {
			statusByNode[t.NodeID] = t.Status
		}
	}
	// 主控是制品源：预取阶段直接视为已就绪，不依赖 agent 自拉。
	for _, t := range targets {
		n, err := s.db.GetNode(t.NodeID)
		if err != nil || n == nil || !n.IsController {
			continue
		}
		if statusByNode[t.NodeID] != core.UpdateCompleted && statusByNode[t.NodeID] != core.UpdateInstallFailed {
			statusByNode[t.NodeID] = core.UpdateStaged
			_ = s.db.UpdateTargetStatus(t.ID, core.UpdateStaged, "")
		}
	}

	s.updateMu.Lock()
	s.activeUpdate = &activeUpdateJob{
		JobID: jobID, TargetVersion: version, Phase: "prefetch",
		Files: files, StatusByNode: statusByNode,
	}
	s.updateMu.Unlock()
	s.notify("updates", map[string]any{"job_id": jobID, "phase": "prefetch"})
	s.broadcastUpdateProgress(jobID)

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		pending := 0
		for _, t := range targets {
			n, err := s.db.GetNode(t.NodeID)
			if err != nil || n == nil || n.IsController {
				continue
			}
			s.updateMu.Lock()
			st := ""
			if s.activeUpdate != nil {
				st = s.activeUpdate.StatusByNode[t.NodeID]
			}
			s.updateMu.Unlock()
			if st == "" {
				st = t.Status
			}
			switch st {
			case core.UpdateStaged, core.UpdateCompleted, core.UpdateInstallFailed,
				core.UpdateDownloadFailed, core.UpdateSignatureInvalid, core.UpdateRolledBack:
				continue
			default:
				pending++
			}
		}
		s.broadcastUpdateProgress(jobID)
		if pending == 0 {
			break
		}
		time.Sleep(2 * time.Second)
		targets, _ = s.db.ListUpdateTargets(jobID)
	}
	// Mark still-pending prefetch targets as failed so install can proceed for the rest.
	targets, _ = s.db.ListUpdateTargets(jobID)
	for _, t := range targets {
		n, _ := s.db.GetNode(t.NodeID)
		if n != nil && n.IsController {
			continue
		}
		s.updateMu.Lock()
		st := ""
		if s.activeUpdate != nil {
			st = s.activeUpdate.StatusByNode[t.NodeID]
		}
		s.updateMu.Unlock()
		if st == "" {
			st = t.Status
		}
		if st != core.UpdateStaged && st != core.UpdateCompleted &&
			st != core.UpdateInstallFailed && st != core.UpdateDownloadFailed {
			s.setUpdateNodeStatus(jobID, t.NodeID, t.ID, core.UpdateDownloadFailed, "prefetch timeout")
		}
	}

	s.updateMu.Lock()
	if s.activeUpdate != nil {
		s.activeUpdate.Phase = "install"
		s.activeUpdate.InstallNodeID = ""
	}
	s.updateMu.Unlock()
	s.notify("updates", map[string]any{"job_id": jobID, "phase": "install"})
	s.broadcastUpdateProgress(jobID)

	targets, _ = s.db.ListUpdateTargets(jobID)
	needControllerRestart := false
	for _, t := range targets {
		n, _ := s.db.GetNode(t.NodeID)
		if n != nil && n.IsController {
			continue // controller last
		}
		s.updateMu.Lock()
		st := ""
		if s.activeUpdate != nil {
			st = s.activeUpdate.StatusByNode[t.NodeID]
		}
		s.updateMu.Unlock()
		if st == "" {
			st = t.Status
		}
		if st == core.UpdateCompleted || st == core.UpdateInstallFailed || st == core.UpdateRolledBack ||
			st == core.UpdateDownloadFailed || st == core.UpdateSignatureInvalid {
			continue
		}
		if st != core.UpdateStaged && st != core.UpdateInstalling {
			s.setUpdateNodeStatus(jobID, t.NodeID, t.ID, core.UpdateInstallFailed, "not staged")
			continue
		}

		// Serial leaf-first: only this node may install now.
		s.updateMu.Lock()
		if s.activeUpdate != nil {
			s.activeUpdate.InstallNodeID = t.NodeID
		}
		s.updateMu.Unlock()
		s.broadcastUpdateProgress(jobID)

		waitUntil := time.Now().Add(5 * time.Minute)
		finished := false
		for time.Now().Before(waitUntil) {
			s.updateMu.Lock()
			st = ""
			if s.activeUpdate != nil {
				st = s.activeUpdate.StatusByNode[t.NodeID]
			}
			s.updateMu.Unlock()
			if st == core.UpdateCompleted || st == core.UpdateInstallFailed || st == core.UpdateRolledBack {
				finished = true
				break
			}
			time.Sleep(1 * time.Second)
			s.broadcastUpdateProgress(jobID)
		}
		if !finished {
			s.setUpdateNodeStatus(jobID, t.NodeID, t.ID, core.UpdateInstallFailed, "install timeout")
		}
		s.updateMu.Lock()
		if s.activeUpdate != nil {
			s.activeUpdate.InstallNodeID = ""
		}
		s.updateMu.Unlock()
	}

	// Controller last: apply bins then restart after job is persisted Completed.
	for _, t := range targets {
		n, _ := s.db.GetNode(t.NodeID)
		if n == nil || !n.IsController {
			continue
		}
		s.updateMu.Lock()
		st := ""
		if s.activeUpdate != nil {
			st = s.activeUpdate.StatusByNode[t.NodeID]
		}
		s.updateMu.Unlock()
		if st == core.UpdateCompleted {
			needControllerRestart = true
			break
		}
		s.setUpdateNodeStatus(jobID, t.NodeID, t.ID, core.UpdateInstalling, "")
		if err := s.installControllerRelease(version); err != nil {
			log.Printf("controller install %s: %v", version, err)
			s.setUpdateNodeStatus(jobID, t.NodeID, t.ID, core.UpdateInstallFailed, err.Error())
		} else {
			_ = s.db.TouchNodeSeen(n.ID, version, n.ActiveGeneration)
			s.setUpdateNodeStatus(jobID, t.NodeID, t.ID, core.UpdateCompleted, "")
			needControllerRestart = true
		}
		s.broadcastUpdateProgress(jobID)
	}

	_ = s.db.UpdateJobStatus(jobID, "Completed")
	// Keep final statuses visible: leave StatusByNode, clear only InstallNodeID / phase marker via nil job after snapshot.
	finalStatuses := map[string]string{}
	s.updateMu.Lock()
	if s.activeUpdate != nil {
		for k, v := range s.activeUpdate.StatusByNode {
			finalStatuses[k] = v
		}
	}
	s.activeUpdate = nil
	s.updateMu.Unlock()
	s.notify("updates", map[string]any{
		"job_id": jobID, "status": "Completed", "phase": "",
		"status_by_node": finalStatuses,
	})
	s.broadcastUpdateProgress(jobID)

	if needControllerRestart {
		log.Printf("update job %s finished; restarting controller to load %s", jobID, version)
		s.scheduleControllerRestart()
	}
}

func (s *Server) setUpdateNodeStatus(jobID, nodeID, targetID, status, errMsg string) {
	if targetID != "" {
		_ = s.db.UpdateTargetStatus(targetID, status, errMsg)
	}
	s.updateMu.Lock()
	if s.activeUpdate != nil && s.activeUpdate.JobID == jobID {
		if s.activeUpdate.StatusByNode == nil {
			s.activeUpdate.StatusByNode = map[string]string{}
		}
		s.activeUpdate.StatusByNode[nodeID] = status
	}
	s.updateMu.Unlock()
}

// resumeOrFinalizeUpdates continues a Running job after controller restart, or closes it if done.
func (s *Server) resumeOrFinalizeUpdates() {
	time.Sleep(2 * time.Second)
	jobs, err := s.db.ListUpdateJobs()
	if err != nil {
		return
	}
	for _, j := range jobs {
		if j.Status != "Running" {
			continue
		}
		targets, err := s.db.ListUpdateTargets(j.ID)
		if err != nil {
			continue
		}
		pending := false
		for _, t := range targets {
			switch t.Status {
			case core.UpdateCompleted, core.UpdateInstallFailed, core.UpdateRolledBack,
				core.UpdateDownloadFailed, core.UpdateSignatureInvalid, core.UpdateHealthFailed:
				continue
			default:
				pending = true
			}
		}
		if !pending {
			_ = s.db.UpdateJobStatus(j.ID, "Completed")
			log.Printf("finalized update job %s after restart (all targets terminal)", j.ID)
			s.notify("updates", map[string]any{"job_id": j.ID, "status": "Completed"})
			return
		}
		s.updateMu.Lock()
		busy := s.activeUpdate != nil
		s.updateMu.Unlock()
		if busy {
			return
		}
		log.Printf("resuming update job %s (%s)", j.ID, j.TargetVersion)
		go s.runUpdateJob(j.ID, j.TargetVersion, targets)
		return
	}
}

func (s *Server) broadcastUpdateProgress(jobID string) {
	s.updateMu.Lock()
	phase := ""
	statusByNode := map[string]string{}
	version := ""
	if s.activeUpdate != nil {
		phase = s.activeUpdate.Phase
		version = s.activeUpdate.TargetVersion
		for k, v := range s.activeUpdate.StatusByNode {
			statusByNode[k] = v
		}
	}
	s.updateMu.Unlock()
	targets, _ := s.db.ListUpdateTargets(jobID)
	for _, t := range targets {
		if _, ok := statusByNode[t.NodeID]; !ok {
			statusByNode[t.NodeID] = t.Status
		}
	}
	s.notify("updates", map[string]any{
		"job_id": jobID, "phase": phase, "target_version": version,
		"status_by_node": statusByNode, "targets": targets,
	})
}

func (s *Server) handleRollbackUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	targets, _ := s.db.ListUpdateTargets(id)
	for _, t := range targets {
		_ = s.db.UpdateTargetStatus(t.ID, core.UpdateRollingBack, "")
		_ = s.db.UpdateTargetStatus(t.ID, core.UpdateRolledBack, "")
	}
	_ = s.db.UpdateJobStatus(id, "RolledBack")
	writeJSON(w, 200, map[string]string{"status": "rolled_back"})
}

func (s *Server) handleUpdateTargets(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListUpdateTargets(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListAuditLogs(200)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	nodes, _ := s.db.ListNodes()
	links, _ := s.db.ListLinks()
	online := 0
	now := time.Now()
	for _, n := range nodes {
		if n.LastSeenAt != nil && now.Sub(*n.LastSeenAt) < 90*time.Second {
			online++
		}
	}
	failedLinks := 0
	for _, l := range links {
		if l.Status == core.LinkFailed {
			failedLinks++
		}
	}
	writeJSON(w, 200, map[string]any{
		"node_total": len(nodes), "agent_online": online, "overlay_reachable": online,
		"link_total": len(links), "link_failed": failedLinks,
		"version": core.ProductVersion,
	})
}
