package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/artifacts"
	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/netd"
	"github.com/FengYuchen1314/sd-wan-plus/internal/ports"
	"github.com/FengYuchen1314/sd-wan-plus/internal/relay"
	"github.com/FengYuchen1314/sd-wan-plus/internal/updater"
	"github.com/go-chi/chi/v5"
)

type Config struct {
	NodeID      string
	ParentURL   string
	DataDir     string
	ArtifactDir string
	NetdAddr    string
	NodePort    int
	Root        string
	ServeChildren bool
	Interval    time.Duration
}

type Agent struct {
	cfg       Config
	netd      *netd.Client
	deduper   *relay.Deduper
	store     *artifacts.Store
	activeGen *int64
	updater   *updater.Updater
}

func New(cfg Config) *Agent {
	if cfg.Interval == 0 {
		cfg.Interval = 15 * time.Second
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.ArtifactDir == "" {
		cfg.ArtifactDir = filepath.Join(filepath.Dir(cfg.DataDir), "artifacts")
	}
	if cfg.NodePort == 0 {
		cfg.NodePort = ports.Node
	}
	if cfg.Root == "" {
		cfg.Root = filepath.Dir(cfg.DataDir)
		if cfg.Root == "." || cfg.Root == "" {
			cfg.Root = "/opt/pathweaver"
		}
	}
	return &Agent{
		cfg:     cfg,
		netd:    &netd.Client{Addr: cfg.NetdAddr},
		deduper: relay.NewDeduper(10 * time.Minute),
		store:   artifacts.New(cfg.ArtifactDir, cfg.ParentURL),
		updater: updater.New(cfg.Root),
	}
}

func (a *Agent) Run() error {
	_ = os.MkdirAll(a.cfg.DataDir, 0o755)
	_ = os.MkdirAll(a.cfg.ArtifactDir, 0o755)
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
	// Seed local cache from installed binaries so children can install from this node
	binDir := filepath.Join(a.cfg.Root, "bin")
	_ = a.store.Seed(binDir, artifacts.NodeBinaries)
	_ = a.store.Seed(a.cfg.ArtifactDir, artifacts.NodeBinaries)

	if a.cfg.ServeChildren {
		go a.serveChildren()
	}

	log.Printf("agent starting node=%s parent=%s childPort=%d serveChildren=%v", a.cfg.NodeID, a.cfg.ParentURL, a.cfg.NodePort, a.cfg.ServeChildren)
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()
	a.tick()
	for range ticker.C {
		a.tick()
	}
	return nil
}

func (a *Agent) serveChildren() {
	r := chi.NewRouter()
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "role": "parent-proxy"})
	})
	r.Get("/bootstrap/install.sh", a.handleChildInstall)
	r.Get("/bootstrap/artifact/{name}", a.handleChildArtifact)
	r.Handle("/bootstrap/enroll", artifacts.ProxyHandler(a.cfg.ParentURL))
	r.Handle("/bootstrap/commit", artifacts.ProxyHandler(a.cfg.ParentURL))
	// Relay agent APIs toward controller via parent chain
	r.Handle("/api/agent/*", artifacts.ProxyHandler(a.cfg.ParentURL))

	addr := fmt.Sprintf(":%d", a.cfg.NodePort)
	log.Printf("parent proxy listening on %s (artifacts + enroll relay)", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Printf("child server error: %v", err)
	}
}

func (a *Agent) handleChildInstall(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", 400)
		return
	}
	// Validate via parent enroll path lightly: fetch script from parent then rewrite BASE to this host
	parentScriptURL := a.cfg.ParentURL + "/bootstrap/install.sh?token=" + urlQueryEscape(token)
	resp, err := http.Get(parentScriptURL)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		// Fallback: local script with this host as BASE
		host := r.Host
		base := "http://" + host
		name := r.URL.Query().Get("name")
		if name == "" {
			name = "node"
		}
		script := artifacts.InstallScript(token, base, name, core.ProductVersion, a.cfg.NodePort)
		w.Header().Set("Content-Type", "text/x-shellscript")
		_, _ = io.WriteString(w, script)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// Force BASE to this parent so child never talks to controller/GitHub
	base := "http://" + r.Host
	rewritten := rewriteInstallBase(string(body), base)
	w.Header().Set("Content-Type", "text/x-shellscript")
	_, _ = io.WriteString(w, rewritten)
}

func rewriteInstallBase(script, base string) string {
	// Replace BASE="..." assignment produced by controller
	lines := strings.Split(script, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "BASE=") {
			lines[i] = fmt.Sprintf("BASE=%q", base)
		}
	}
	return strings.Join(lines, "\n")
}

func urlQueryEscape(s string) string {
	return strings.ReplaceAll(s, " ", "%20")
}

func (a *Agent) handleChildArtifact(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	a.store.ServeHTTP(w, r, name)
}

func (a *Agent) tick() {
	a.sendHeartbeat()
	a.pullAndApply()
	a.pollUpdate()
}

func (a *Agent) sendHeartbeat() {
	body := map[string]any{
		"node_id":           a.cfg.NodeID,
		"agent_version":     core.ProductVersion,
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
	a.injectLocalWGKey(&st)
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

func (a *Agent) injectLocalWGKey(st *core.NodeDesiredState) {
	path := filepath.Join(a.cfg.DataDir, "wg_private.key")
	b, err := os.ReadFile(path)
	key := ""
	if err == nil {
		key = strings.TrimSpace(string(b))
	}
	for i := range st.WireGuardLinks {
		if strings.TrimSpace(st.WireGuardLinks[i].NodePrivateKey) == "" && key != "" {
			st.WireGuardLinks[i].NodePrivateKey = key
		}
		if key == "" && strings.TrimSpace(st.WireGuardLinks[i].NodePrivateKey) != "" {
			key = strings.TrimSpace(st.WireGuardLinks[i].NodePrivateKey)
		}
	}
	if key != "" {
		_ = os.WriteFile(path, []byte(key+"\n"), 0o600)
	}
}

type pendingUpdate struct {
	JobID         string   `json:"job_id"`
	TargetVersion string   `json:"target_version"`
	Phase         string   `json:"phase"` // prefetch|install
	Files         []string `json:"files"`
	Depth         int      `json:"depth"`
}

func (a *Agent) pollUpdate() {
	url := fmt.Sprintf("%s/api/agent/update/%s", a.cfg.ParentURL, a.cfg.NodeID)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	var u pendingUpdate
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil || u.JobID == "" {
		return
	}
	files := u.Files
	if len(files) == 0 {
		files = artifacts.NodeBinaries
	}
	switch u.Phase {
	case "prefetch":
		log.Printf("update prefetch %s from parent", u.TargetVersion)
		_ = a.reportUpdate(u.JobID, core.UpdatePrefetching, "")
		if err := a.prefetch(u.TargetVersion, files); err != nil {
			_ = a.reportUpdate(u.JobID, core.UpdateDownloadFailed, err.Error())
			return
		}
		_ = a.reportUpdate(u.JobID, core.UpdateStaged, "")
	case "install":
		log.Printf("update install %s", u.TargetVersion)
		_ = a.reportUpdate(u.JobID, core.UpdateInstalling, "")
		relDir := filepath.Join(a.cfg.ArtifactDir, "releases", u.TargetVersion)
		man := &updater.Manifest{
			Version: u.TargetVersion, ProtocolVersion: core.ProtocolVersion,
			Files: map[string]string{},
		}
		for _, f := range files {
			man.Files[f] = "" // hash optional when empty skipped in Stage if we adjust
		}
		if err := a.updater.StageFromDir(u.TargetVersion, relDir); err != nil {
			_ = a.reportUpdate(u.JobID, core.UpdateInstallFailed, err.Error())
			return
		}
		if err := a.updater.Activate(u.TargetVersion); err != nil {
			_ = a.reportUpdate(u.JobID, core.UpdateInstallFailed, err.Error())
			return
		}
		_ = a.reportUpdate(u.JobID, core.UpdateCompleted, "")
		a.scheduleServiceRestart()
	}
}

func (a *Agent) scheduleServiceRestart() {
	go func() {
		time.Sleep(2 * time.Second)
		units := []string{"pathweaver-netd", "pathweaver-updater", "pathweaver-agent"}
		// 控制机还要重启主控（安装脚本嵌在 controller 进程里）
		if _, err := os.Stat(filepath.Join(a.cfg.Root, "bin", "pathweaver-controller")); err == nil {
			if exec.Command("systemctl", "cat", "pathweaver").Run() == nil {
				units = append([]string{"pathweaver"}, units...)
			}
		}
		log.Printf("restarting services after update: %v", units)
		args := append([]string{"restart"}, units...)
		_ = exec.Command("systemctl", args...).Run()
	}()
}

func (a *Agent) prefetch(version string, files []string) error {
	relDir := filepath.Join(a.cfg.ArtifactDir, "releases", version)
	_ = os.MkdirAll(relDir, 0o755)
	for _, name := range files {
		name = filepath.Base(name)
		p, err := a.store.Ensure(name)
		if err != nil {
			if artifacts.OptionalArtifacts[name] {
				log.Printf("prefetch skip optional %s: %v", name, err)
				continue
			}
			return fmt.Errorf("%s: %w", name, err)
		}
		dst := filepath.Join(relDir, name)
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
		// also keep top-level cache for child bootstrap
		_ = copyFileSimple(p, a.store.Path(name))
	}
	return nil
}

func copyFileSimple(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (a *Agent) reportUpdate(jobID, status, errMsg string) error {
	body, _ := json.Marshal(map[string]string{
		"node_id": a.cfg.NodeID, "job_id": jobID, "status": status, "error": errMsg,
	})
	resp, err := http.Post(a.cfg.ParentURL+"/api/agent/update/report", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

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

func ParseNodePort(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return ports.Node
	}
	return n
}
