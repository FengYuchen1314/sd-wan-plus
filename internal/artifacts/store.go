package artifacts

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Required node install / update binaries served from every parent.
var NodeBinaries = []string{
	"pathweaver-agent",
	"pathweaver-netd",
	"pathweaver-updater",
	"pathweaver-cli",
	"pathweaver-controller",
	"install-node.sh",
}

// OptionalArtifacts may be absent; prefetch should not fail the whole job.
var OptionalArtifacts = map[string]bool{
	"manifest.json":     true,
	"install-node.sh":   true,
	"pathweaver-cli":    true,
	"pathweaver-controller": true,
}

// Store is a local artifact cache. Misses are fetched from ParentURL.
type Store struct {
	Dir       string
	ParentURL string // empty on controller origin cache
	mu        sync.Mutex
	client    *http.Client
}

func New(dir, parentURL string) *Store {
	_ = os.MkdirAll(dir, 0o755)
	return &Store{Dir: dir, ParentURL: strings.TrimRight(parentURL, "/"), client: &http.Client{}}
}

func (s *Store) Path(name string) string {
	return filepath.Join(s.Dir, filepath.Base(name))
}

func (s *Store) Has(name string) bool {
	st, err := os.Stat(s.Path(name))
	return err == nil && !st.IsDir() && st.Size() > 0
}

// Seed copies files from srcDir into the cache (controller install / release stage).
func (s *Store) Seed(srcDir string, names []string) error {
	for _, name := range names {
		src := filepath.Join(srcDir, name)
		// also try .exe on Windows build hosts
		if _, err := os.Stat(src); err != nil {
			alt := src + ".exe"
			if _, err2 := os.Stat(alt); err2 == nil {
				src = alt
				name = name // keep logical name without .exe for Linux nodes
			} else {
				continue
			}
		}
		dst := s.Path(name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("seed %s: %w", name, err)
		}
	}
	return nil
}

// SeedRelease copies a whole release directory into cache under releases/<ver>/.
func (s *Store) SeedRelease(version, srcDir string) error {
	dest := filepath.Join(s.Dir, "releases", version)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(srcDir, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return err
		}
		// also mirror top-level binaries for bootstrap
		base := e.Name()
		for _, n := range NodeBinaries {
			if base == n || base == n+".exe" {
				_ = copyFile(filepath.Join(srcDir, e.Name()), s.Path(n))
			}
		}
	}
	return nil
}

// Ensure returns a local path, fetching from parent if missing.
func (s *Store) Ensure(name string) (string, error) {
	name = filepath.Base(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.Path(name)
	if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
		return p, nil
	}
	if s.ParentURL == "" {
		return "", fmt.Errorf("artifact %s not in local cache and no parent", name)
	}
	url := s.ParentURL + "/bootstrap/artifact/" + name
	resp, err := s.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("parent returned %d for %s", resp.StatusCode, name)
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return p, nil
}

// EnsureRelease fetches all files listed in parent manifest for a version.
func (s *Store) EnsureRelease(version string, files []string) error {
	relDir := filepath.Join(s.Dir, "releases", version)
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		return err
	}
	for _, name := range files {
		name = filepath.Base(name)
		dst := filepath.Join(relDir, name)
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			continue
		}
		// fetch via bootstrap artifact path releases/<ver>/<name> or flat name
		p, err := s.Ensure("releases/" + version + "/" + name)
		if err != nil {
			// fallback flat name then copy into release dir
			p, err = s.Ensure(name)
			if err != nil {
				return err
			}
		}
		if p != dst {
			if err := copyFile(p, dst); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request, name string) {
	name = filepath.Base(name)
	// allow releases/v/file via nested path encoded as releases--v--file? keep simple: basename only for bootstrap
	p, err := s.Ensure(name)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	http.ServeFile(w, r, p)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
