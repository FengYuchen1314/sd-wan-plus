package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
)

type Manifest struct {
	Version           string            `json:"version"`
	ProtocolVersion   int               `json:"protocol_version"`
	Architecture      string            `json:"architecture"`
	Files             map[string]string `json:"files"` // name -> sha256
	MinCompatVersion  string            `json:"min_compat_version"`
	GitCommit         string            `json:"git_commit"`
	BuiltAt           string            `json:"built_at"`
	AllowDowngrade    bool              `json:"allow_downgrade"`
}

type Updater struct {
	Root       string // /opt/pathweaver
	PubKeyB64  string // Ed25519 release public key
	CurrentVer string
}

func New(root string) *Updater {
	if root == "" {
		root = "/opt/pathweaver"
	}
	return &Updater{Root: root, CurrentVer: core.ProductVersion}
}

func (u *Updater) Stage(version string, artifactDir string, manifest *Manifest, sigB64 string) error {
	if u.PubKeyB64 != "" {
		raw, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		if !security.VerifyEd25519(u.PubKeyB64, sigB64, raw) {
			return core.NewError(core.ErrSignatureInvalid, "manifest signature invalid")
		}
	}
	if manifest.Architecture != "" && manifest.Architecture != runtime.GOARCH {
		return fmt.Errorf("arch mismatch: want %s got %s", runtime.GOARCH, manifest.Architecture)
	}
	for name, want := range manifest.Files {
		p := filepath.Join(artifactDir, name)
		if want == "" {
			if _, err := os.Stat(p); err != nil {
				return fmt.Errorf("missing file %s", name)
			}
			continue
		}
		got, err := security.SHA256File(p)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("hash mismatch for %s", name)
		}
	}
	dest := filepath.Join(u.Root, "releases", version)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for name := range manifest.Files {
		src := filepath.Join(artifactDir, name)
		dst := filepath.Join(dest, name)
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// StageFromDir copies all files from dir into releases/<version> without manifest hashes.
func (u *Updater) StageFromDir(version, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	dest := filepath.Join(u.Root, "releases", version)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	bin := filepath.Join(u.Root, "bin")
	_ = os.MkdirAll(bin, 0o755)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(dir, e.Name())
		if err := copyFile(src, filepath.Join(dest, e.Name())); err != nil {
			return err
		}
		// stage into bin for next restart
		_ = copyFile(src, filepath.Join(bin, e.Name()))
	}
	return nil
}

func (u *Updater) Activate(version string) error {
	current := filepath.Join(u.Root, "current")
	target := filepath.Join(u.Root, "releases", version)
	if _, err := os.Stat(target); err != nil {
		return err
	}
	tmp := current + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		// Windows may not support symlink without privileges — fallback copy marker
		return os.WriteFile(current+".txt", []byte(version), 0o644)
	}
	return os.Rename(tmp, current)
}

func (u *Updater) Rollback(prevVersion string) error {
	return u.Activate(prevVersion)
}

func (u *Updater) HealthCheck() error {
	// Basic: binaries exist under current
	cur := filepath.Join(u.Root, "current")
	if _, err := os.Stat(cur); err != nil {
		// marker file mode
		if _, err2 := os.Stat(cur + ".txt"); err2 != nil {
			return err
		}
	}
	return nil
}

func (u *Updater) RunLoop() error {
	parent := os.Getenv("PW_PARENT_URL")
	artDir := os.Getenv("PW_ARTIFACT_DIR")
	if artDir == "" {
		artDir = filepath.Join(u.Root, "artifacts")
	}
	log.Printf("updater running root=%s version=%s parent=%s", u.Root, u.CurrentVer, parent)
	// Updater cooperates with agent: agent stages files; updater watches current.txt / releases
	for {
		time.Sleep(30 * time.Second)
		_ = artDir
	}
}

func copyFile(src, dst string) error {
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
	h := sha256.New()
	w := io.MultiWriter(out, h)
	if _, err := io.Copy(w, in); err != nil {
		return err
	}
	_ = hex.EncodeToString(h.Sum(nil))
	return out.Close()
}
