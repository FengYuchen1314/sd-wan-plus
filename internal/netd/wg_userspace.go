package netd

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

const wgMTU = 1420

type liveDevice struct {
	name string
	dev  *device.Device
	fp   string
}

// wgManager keeps wireguard-go devices alive across activate calls.
type wgManager struct {
	mu   sync.Mutex
	devs map[string]*liveDevice
}

func newWGManager() *wgManager {
	return &wgManager{devs: map[string]*liveDevice{}}
}

func (m *wgManager) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, ld := range m.devs {
		ld.dev.Close()
		delete(m.devs, name)
	}
}

// reconcile ensures devices match desired links: create/hot-update/remove.
func (m *wgManager) reconcile(links []core.WireGuardLinkCfg) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	want := map[string]core.WireGuardLinkCfg{}
	for _, l := range links {
		if l.InterfaceName == "" {
			continue
		}
		want[l.InterfaceName] = l
	}

	for name, ld := range m.devs {
		if _, ok := want[name]; !ok {
			log.Printf("netd: closing wireguard-go device %s", name)
			ld.dev.Close()
			delete(m.devs, name)
			_ = run("ip", "link", "del", name)
		}
	}

	for name, l := range want {
		uapi, err := buildUAPI(l)
		if err != nil {
			return fmt.Errorf("link %s: %w", name, err)
		}
		fp := fingerprint(uapi)
		if ld, ok := m.devs[name]; ok {
			if ld.fp == fp {
				continue
			}
			log.Printf("netd: hot-updating wireguard-go %s", name)
			if err := ld.dev.IpcSet(uapi); err != nil {
				return fmt.Errorf("IpcSet %s: %w", name, err)
			}
			ld.fp = fp
			continue
		}
		log.Printf("netd: creating wireguard-go device %s", name)
		// Drop leftover kernel/userspace iface with same name if any
		_ = run("ip", "link", "del", name)
		tunDev, err := tun.CreateTUN(name, wgMTU)
		if err != nil {
			return fmt.Errorf("CreateTUN %s: %w", name, err)
		}
		if realName, err := tunDev.Name(); err == nil && realName != "" && realName != name {
			_ = tunDev.Close()
			return fmt.Errorf("CreateTUN: requested %q got %q (IFNAMSIZ limit?)", name, realName)
		}
		logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", name))
		dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)
		if err := dev.IpcSet(uapi); err != nil {
			dev.Close()
			return fmt.Errorf("IpcSet %s: %w", name, err)
		}
		if err := dev.Up(); err != nil {
			dev.Close()
			return fmt.Errorf("Up %s: %w", name, err)
		}
		m.devs[name] = &liveDevice{name: name, dev: dev, fp: fp}
	}
	return nil
}

func fingerprint(uapi string) string {
	sum := sha256.Sum256([]byte(uapi))
	return hex.EncodeToString(sum[:8])
}

func keyToHex(b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return "", fmt.Errorf("empty key")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("want 32-byte key, got %d", len(raw))
	}
	return hex.EncodeToString(raw), nil
}

func buildUAPI(l core.WireGuardLinkCfg) (string, error) {
	priv, err := keyToHex(l.NodePrivateKey)
	if err != nil {
		return "", fmt.Errorf("private key: %w", err)
	}
	pub, err := keyToHex(l.PeerPublicKey)
	if err != nil {
		return "", fmt.Errorf("peer public key: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", priv)
	if l.ListenPort > 0 {
		fmt.Fprintf(&b, "listen_port=%d\n", l.ListenPort)
	}
	b.WriteString("replace_peers=true\n")
	fmt.Fprintf(&b, "public_key=%s\n", pub)
	ips := l.AllowedIPs
	seen := map[string]bool{}
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || seen[ip] {
			continue
		}
		if !strings.Contains(ip, "/") {
			ip += "/32"
		}
		seen[ip] = true
		fmt.Fprintf(&b, "allowed_ip=%s\n", ip)
	}
	endpoint := strings.TrimSpace(l.PeerEndpoint)
	if endpoint != "" {
		if resolved, err := resolveEndpoint(endpoint); err == nil {
			endpoint = resolved
		}
		fmt.Fprintf(&b, "endpoint=%s\n", endpoint)
		ka := l.PersistentKeepalive
		if ka == 0 {
			ka = 25
		}
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", ka)
	} else if l.IsInitiator {
		return "", fmt.Errorf("initiator link %s missing peer endpoint", l.LinkID)
	}
	return b.String(), nil
}

func resolveEndpoint(ep string) (string, error) {
	host, port, err := net.SplitHostPort(ep)
	if err != nil {
		return ep, err
	}
	if ip := net.ParseIP(host); ip != nil {
		return ep, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return ep, fmt.Errorf("resolve %s: %v", host, err)
	}
	// Prefer IPv4
	var chosen net.IP
	for _, ip := range ips {
		if ip.To4() != nil {
			chosen = ip.To4()
			break
		}
	}
	if chosen == nil {
		chosen = ips[0]
	}
	return net.JoinHostPort(chosen.String(), port), nil
}
