package controller

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/artifacts"
	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/topology"
)

const defaultUpdateRepo = "FengYuchen1314/sd-wan-plus"

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

type latestReleaseInfo struct {
	Version        string `json:"version"`
	Tag            string `json:"tag"`
	PublishedAt    string `json:"published_at"`
	AssetName      string `json:"asset_name"`
	DownloadURL    string `json:"download_url"`
	CurrentVersion string `json:"current_version"`
	Outdated       bool   `json:"outdated"`
	Commit         string `json:"commit,omitempty"`
}

func (s *Server) updateRepo() string {
	if v := os.Getenv("PW_REPO"); v != "" {
		return v
	}
	return defaultUpdateRepo
}

func (s *Server) fetchGitHubLatest() (*latestReleaseInfo, error) {
	return s.fetchGitHubLatestCached(false)
}

func (s *Server) fetchGitHubLatestCached(force bool) (*latestReleaseInfo, error) {
	const ttl = 30 * time.Minute
	s.ghMu.Lock()
	if !force && s.ghCached != nil && time.Since(s.ghCachedAt) < ttl {
		info := *s.ghCached
		s.ghMu.Unlock()
		return &info, nil
	}
	if !force && s.ghCachedErr != "" && time.Since(s.ghCachedAt) < 5*time.Minute {
		errMsg := s.ghCachedErr
		cached := s.ghCached
		s.ghMu.Unlock()
		if cached != nil {
			cp := *cached
			return &cp, nil
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	s.ghMu.Unlock()

	info, err := s.fetchGitHubLatestUncached()
	s.ghMu.Lock()
	defer s.ghMu.Unlock()
	s.ghCachedAt = time.Now()
	if err != nil {
		s.ghCachedErr = err.Error()
		// keep previous good cache if any
		if s.ghCached != nil {
			cp := *s.ghCached
			return &cp, nil
		}
		// fallback: direct download URL without API
		fb := s.directLatestFallback()
		s.ghCached = fb
		return fb, nil
	}
	s.ghCachedErr = ""
	s.ghCached = info
	cp := *info
	return &cp, nil
}

func (s *Server) directLatestFallback() *latestReleaseInfo {
	repo := s.updateRepo()
	goarch := runtime.GOARCH
	asset := fmt.Sprintf("pathweaver-linux-%s.tar.gz", goarch)
	cur := core.ProductVersion
	return &latestReleaseInfo{
		Version: cur, Tag: "latest", PublishedAt: "",
		AssetName: asset,
		DownloadURL: fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, asset),
		CurrentVersion: cur, Outdated: false,
	}
}

func (s *Server) fetchGitHubLatestUncached() (*latestReleaseInfo, error) {
	repo := s.updateRepo()
	api := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PathWeaver-Controller")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github api %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	goarch := runtime.GOARCH
	stable := fmt.Sprintf("pathweaver-linux-%s.tar.gz", goarch)
	var downloadURL, assetName string
	version := ""
	// Date+time versions: pathweaver-2026.07.20-1511-linux-amd64.tar.gz (same-day builds distinguishable)
	verRe := regexp.MustCompile(`pathweaver-(\d{4}\.\d{2}\.\d{2}(?:-\d{4})?)-linux-` + regexp.QuoteMeta(goarch) + `\.tar\.gz`)
	legacyVerRe := regexp.MustCompile(`pathweaver-(.+)-linux-` + regexp.QuoteMeta(goarch) + `\.tar\.gz`)
	commit := ""
	if m := regexp.MustCompile(`(?i)Commit:\s*([0-9a-f]{7,40})`).FindStringSubmatch(rel.Body); len(m) == 2 {
		commit = m[1]
	}
	for _, a := range rel.Assets {
		if a.Name == stable {
			downloadURL = a.BrowserDownloadURL
			assetName = a.Name
		}
		if m := verRe.FindStringSubmatch(a.Name); len(m) == 2 {
			if m[1] > version {
				version = m[1]
				if downloadURL == "" {
					downloadURL = a.BrowserDownloadURL
					assetName = a.Name
				}
			}
		}
	}
	if version == "" {
		for _, a := range rel.Assets {
			if m := legacyVerRe.FindStringSubmatch(a.Name); len(m) == 2 && !strings.HasPrefix(m[1], "linux") {
				version = m[1]
				if downloadURL == "" {
					downloadURL = a.BrowserDownloadURL
					assetName = a.Name
				}
				break
			}
		}
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("release 中未找到 linux-%s 安装包", goarch)
	}
	if version == "" {
		version = time.Now().UTC().Format("2006.01.02-1504")
	}

	cur := core.ProductVersion
	outdated := version != "" && version != cur

	return &latestReleaseInfo{
		Version: version, Tag: rel.TagName, PublishedAt: rel.PublishedAt,
		AssetName: assetName, DownloadURL: downloadURL,
		CurrentVersion: cur, Outdated: outdated,
		Commit: commit,
	}, nil
}

func (s *Server) handleUpdatesLatest(w http.ResponseWriter, r *http.Request) {
	info, err := s.fetchGitHubLatest()
	if err != nil {
		writeJSON(w, 502, map[string]string{"message": "拉取 GitHub Release 失败: " + err.Error()})
		return
	}
	writeJSON(w, 200, info)
}

func (s *Server) handleUpdatesOverview(w http.ResponseWriter, r *http.Request) {
	jobs, _ := s.db.ListUpdateJobs()
	nodes, _ := s.db.ListNodes()
	g, _ := topology.Build(s.db)

	var activeJob *core.UpdateJob
	var targets []core.UpdateTarget
	phase := ""
	statusByNode := map[string]string{}

	s.updateMu.Lock()
	if s.activeUpdate != nil {
		phase = s.activeUpdate.Phase
		for k, v := range s.activeUpdate.StatusByNode {
			statusByNode[k] = v
		}
		for i := range jobs {
			if jobs[i].ID == s.activeUpdate.JobID {
				activeJob = &jobs[i]
				break
			}
		}
	}
	s.updateMu.Unlock()

	if activeJob == nil {
		for i := range jobs {
			if jobs[i].Status == "Running" {
				activeJob = &jobs[i]
				break
			}
		}
	}
	if activeJob != nil {
		targets, _ = s.db.ListUpdateTargets(activeJob.ID)
		for _, t := range targets {
			if _, ok := statusByNode[t.NodeID]; !ok {
				statusByNode[t.NodeID] = t.Status
			}
		}
	}

	now := time.Now()
	nodeRows := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		online := n.LastSeenAt != nil && now.Sub(*n.LastSeenAt) < 90*time.Second
		depth, _ := s.db.NodeDepth(n.ID)
		st := statusByNode[n.ID]
		if st == "" {
			st = "Idle"
		}
		errMsg := ""
		for _, t := range targets {
			if t.NodeID == n.ID {
				errMsg = t.ErrorMessage
				break
			}
		}
		nodeRows = append(nodeRows, map[string]any{
			"id": n.ID, "display_name": n.DisplayName, "is_controller": n.IsController,
			"agent_version": n.AgentVersion, "online": online, "depth": depth,
			"update_status": st, "error_message": errMsg,
			"needs_update": activeJob != nil && n.AgentVersion != activeJob.TargetVersion && st != core.UpdateCompleted,
		})
	}

	var latest any
	forceGitHub := r.URL.Query().Get("refresh_github") == "1"
	if r.URL.Query().Get("skip_github") == "1" {
		s.ghMu.Lock()
		if s.ghCached != nil {
			cp := *s.ghCached
			latest = &cp
		} else {
			latest = map[string]any{"current_version": core.ProductVersion, "version": core.ProductVersion, "outdated": false}
		}
		s.ghMu.Unlock()
	} else if info, err := s.fetchGitHubLatestCached(forceGitHub); err == nil {
		latest = info
	} else {
		latest = map[string]any{"error": err.Error(), "current_version": core.ProductVersion}
	}

	writeJSON(w, 200, map[string]any{
		"current_version": core.ProductVersion,
		"latest":          latest,
		"active_job":      activeJob,
		"phase":           phase,
		"nodes":           nodeRows,
		"targets":         targets,
		"topology":        g,
		"jobs":            jobs,
	})
}

func (s *Server) handlePullLatest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AutoStart bool `json:"auto_start"`
	}
	_ = readJSON(r, &body)

	info, err := s.fetchGitHubLatestCached(true)
	if err != nil || info == nil || info.DownloadURL == "" {
		info = s.directLatestFallback()
	}

	tmpDir, err := os.MkdirTemp("", "pw-update-*")
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "pkg.tar.gz")
	if err := downloadFile(info.DownloadURL, tarPath); err != nil {
		writeJSON(w, 502, map[string]string{"message": "下载安装包失败: " + err.Error()})
		return
	}
	extractDir := filepath.Join(tmpDir, "extract")
	if err := extractTarGz(tarPath, extractDir); err != nil {
		writeJSON(w, 500, map[string]string{"message": "解压失败: " + err.Error()})
		return
	}
	binDir, err := findPackageBinDir(extractDir)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": err.Error()})
		return
	}

	files := artifacts.NodeBinaries
	pkgRoot := filepath.Dir(binDir)
	if s.store != nil {
		_ = s.store.Seed(binDir, files)
		if err := s.store.SeedRelease(info.Version, binDir); err != nil {
			writeJSON(w, 500, map[string]string{"message": "写入制品缓存失败: " + err.Error()})
			return
		}
		for _, name := range []string{"install.sh", "install-node.sh"} {
			src := filepath.Join(pkgRoot, name)
			if st, err := os.Stat(src); err == nil && !st.IsDir() {
				_ = copyFileSimple(src, filepath.Join(s.store.Dir, "install-node.sh"))
				_ = copyFileSimple(src, filepath.Join(s.store.Dir, "releases", info.Version, "install-node.sh"))
			}
		}
	}

	// 前端可热更新；主控二进制等任务安装阶段末尾再替换并重启，避免中途杀掉更新编排。
	root := filepath.Clean(filepath.Join(s.cfg.DataDir, ".."))
	if root == "" || root == "." || root == "/" {
		root = "/opt/pathweaver"
	}
	webSrc := filepath.Join(pkgRoot, "web")
	if st, err := os.Stat(webSrc); err == nil && st.IsDir() {
		if err := copyDirSimple(webSrc, filepath.Join(root, "web")); err != nil {
			log.Printf("copy web during pull-latest: %v", err)
		}
	}

	job, err := s.db.CreateUpdateJob(info.Version, info.Commit)
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
	_ = s.db.AddAudit(&admin.ID, "pull_latest_update", "update_job", &job.ID, info.Version, clientIP(r))

	started := false
	if body.AutoStart {
		_ = s.db.UpdateJobStatus(job.ID, "Running")
		targets, _ := s.db.ListUpdateTargets(job.ID)
		go s.runUpdateJob(job.ID, job.TargetVersion, targets)
		started = true
	}

	s.notify("updates", map[string]any{
		"job_id": job.ID, "target_version": info.Version, "pulled": true, "started": started,
	})

	note := "已写入制品缓存；叶子节点先更新，主控最后替换二进制并重启"
	writeJSON(w, 200, map[string]any{
		"job": job, "latest": info, "started": started,
		"note": note,
	})
}

// installControllerRelease applies staged release bins onto the controller install root.
func (s *Server) installControllerRelease(version string) error {
	if s.store == nil {
		return fmt.Errorf("artifact store unavailable")
	}
	binDir := filepath.Join(s.store.Dir, "releases", version)
	if st, err := os.Stat(binDir); err != nil || !st.IsDir() {
		return fmt.Errorf("release %s not staged at %s", version, binDir)
	}
	pkgRoot := filepath.Clean(filepath.Join(s.cfg.DataDir, ".."))
	return s.applyLocalRelease(binDir, pkgRoot)
}

// applyLocalRelease copies package bins (+ optional web) into the install root next to DataDir.
func (s *Server) applyLocalRelease(binDir, pkgRoot string) error {
	root := filepath.Clean(filepath.Join(s.cfg.DataDir, ".."))
	if root == "" || root == "." || root == "/" {
		root = "/opt/pathweaver"
	}
	destBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(destBin, 0o755); err != nil {
		return err
	}
	names := append([]string{"pathweaver-controller"}, artifacts.NodeBinaries...)
	seen := map[string]bool{}
	for _, name := range names {
		name = filepath.Base(name)
		if seen[name] || name == "install-node.sh" {
			continue
		}
		seen[name] = true
		src := filepath.Join(binDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := copyFileSimple(src, filepath.Join(destBin, name)); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
	}
	webSrc := filepath.Join(pkgRoot, "web")
	if st, err := os.Stat(webSrc); err == nil && st.IsDir() {
		if err := copyDirSimple(webSrc, filepath.Join(root, "web")); err != nil {
			log.Printf("copy web: %v", err)
		}
	}
	return nil
}

func (s *Server) scheduleControllerRestart() {
	go func() {
		time.Sleep(2 * time.Second)
		log.Printf("restarting pathweaver after local release apply")
		_ = exec.Command("systemctl", "restart", "pathweaver").Run()
	}()
}

func copyDirSimple(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFileSimple(path, target)
	})
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PathWeaver-Controller")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractTarGz(src, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." || name == "" {
			continue
		}
		if strings.HasPrefix(name, "..") || strings.Contains(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func findPackageBinDir(root string) (string, error) {
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || !info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "bin" {
			if _, err := os.Stat(filepath.Join(path, "pathweaver-agent")); err == nil {
				found = path
				return io.EOF
			}
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("安装包内未找到 bin/pathweaver-agent")
	}
	return found, nil
}
