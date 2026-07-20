package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/netd"
	"github.com/FengYuchen1314/sd-wan-plus/internal/relay"
)

type Config struct {
	NodeID     string
	ParentURL  string
	DataDir    string
	NetdAddr   string
	Interval   time.Duration
}

type Agent struct {
	cfg     Config
	netd    *netd.Client
	deduper *relay.Deduper
	activeGen *int64
}

func New(cfg Config) *Agent {
	if cfg.Interval == 0 {
		cfg.Interval = 15 * time.Second
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	return &Agent{
		cfg: cfg,
		netd: &netd.Client{Addr: cfg.NetdAddr},
		deduper: relay.NewDeduper(10 * time.Minute),
	}
}

func (a *Agent) Run() error {
	_ = os.MkdirAll(a.cfg.DataDir, 0o755)
	if a.cfg.NodeID == "" {
		b, err := os.ReadFile(filepath.Join(a.cfg.DataDir, "node_id"))
		if err == nil {
			a.cfg.NodeID = string(bytes.TrimSpace(b))
		}
	}
	if a.cfg.NodeID == "" {
		return fmt.Errorf("PW_NODE_ID or data/node_id required")
	}
	if a.cfg.ParentURL == "" {
		return fmt.Errorf("PW_PARENT_URL required")
	}
	log.Printf("agent starting node=%s parent=%s", a.cfg.NodeID, a.cfg.ParentURL)
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()
	a.tick()
	for range ticker.C {
		a.tick()
	}
	return nil
}

func (a *Agent) tick() {
	a.sendHeartbeat()
	a.pullAndApply()
}

func (a *Agent) sendHeartbeat() {
	body := map[string]any{
		"node_id": a.cfg.NodeID,
		"agent_version": core.ProductVersion,
		"active_generation": a.activeGen,
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(a.cfg.ParentURL+"/api/agent/heartbeat", "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("heartbeat error: %v", err)
		return
	}
	resp.Body.Close()
}

func (a *Agent) pullAndApply() {
	url := fmt.Sprintf("%s/api/agent/desired/%s", a.cfg.ParentURL, a.cfg.NodeID)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	var st core.NodeDesiredState
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		log.Printf("desired decode: %v", err)
		return
	}
	prep, err := a.netd.Call(netd.Request{Action: "prepare", State: &st})
	if err != nil || !prep.OK {
		log.Printf("prepare failed: %v %v", err, prep)
		return
	}
	act, err := a.netd.Call(netd.Request{Action: "activate", State: &st})
	if err != nil || !act.OK {
		log.Printf("activate failed: %v %v", err, act)
		return
	}
	ver, err := a.netd.Call(netd.Request{Action: "verify", State: &st})
	if err != nil || !ver.OK {
		log.Printf("verify failed: %v %v", err, ver)
		_, _ = a.netd.Call(netd.Request{Action: "rollback"})
		return
	}
	g := int64(st.Generation)
	a.activeGen = &g
	_ = os.WriteFile(filepath.Join(a.cfg.DataDir, "active_generation"), []byte(fmt.Sprintf("%d", g)), 0o644)
	log.Printf("applied generation %d hash=%s", st.Generation, st.ConfigHash)
}

// RelayMessage forwards control envelopes toward parent or children.
func (a *Agent) RelayMessage(env *relay.Envelope) error {
	if a.deduper.Seen(env.MessageID) {
		return nil
	}
	if !env.DecrementTTL() {
		return fmt.Errorf("ttl exhausted")
	}
	b, _ := json.Marshal(env)
	resp, err := http.Post(a.cfg.ParentURL+"/api/agent/relay", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
