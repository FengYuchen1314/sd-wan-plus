package netd

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
)

const DefaultSocket = "/run/pathweaver/netd.sock"

// Server applies structured DesiredState via wireguard-go + ip/nft (Linux).
type Server struct {
	Socket   string
	lastGood *core.NodeDesiredState
	wg       *wgManager
}

func New(socket string) *Server {
	if socket == "" {
		socket = DefaultSocket
	}
	return &Server{Socket: socket, wg: newWGManager()}
}

type Request struct {
	Action string                 `json:"action"` // prepare|activate|verify|rollback|status
	State  *core.NodeDesiredState `json:"state,omitempty"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) ListenAndServe() error {
	if runtime.GOOS != "linux" {
		log.Printf("netd: non-linux OS (%s), running in dry-run mode", runtime.GOOS)
	} else {
		log.Printf("netd: data plane = wireguard-go (userspace)")
	}
	_ = os.Remove(s.Socket)
	if err := os.MkdirAll(filepath.Dir(s.Socket), 0o755); err != nil {
		return err
	}
	ln, err := net.Listen("unix", s.Socket)
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:18765")
		if err != nil {
			return err
		}
		log.Printf("netd listening on tcp 127.0.0.1:18765 (unix unavailable)")
	} else {
		log.Printf("netd listening on %s", s.Socket)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.wg.closeAll()
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	var req Request
	if err := dec.Decode(&req); err != nil {
		_ = enc.Encode(Response{OK: false, Error: err.Error()})
		return
	}
	var resp Response
	switch req.Action {
	case "prepare":
		resp = s.prepare(req.State)
	case "activate":
		resp = s.activate(req.State)
	case "verify":
		resp = s.verify(req.State)
	case "rollback":
		resp = s.rollback()
	case "status":
		resp = Response{OK: true, Message: "ok"}
	default:
		resp = Response{OK: false, Error: "unknown action"}
	}
	_ = enc.Encode(resp)
}

func (s *Server) prepare(st *core.NodeDesiredState) Response {
	if st == nil {
		return Response{OK: false, Error: "missing state"}
	}
	if st.ConfigHash == "" {
		return Response{OK: false, Error: "missing config_hash"}
	}
	if st.OverlayIdentity.DummyInterface == "" {
		return Response{OK: false, Error: "missing overlay identity"}
	}
	return Response{OK: true, Message: "prepared"}
}

func (s *Server) activate(st *core.NodeDesiredState) Response {
	if st == nil {
		return Response{OK: false, Error: "missing state"}
	}
	if err := s.apply(st); err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	s.lastGood = st
	return Response{OK: true, Message: "activated"}
}

func (s *Server) verify(st *core.NodeDesiredState) Response {
	if st == nil {
		return Response{OK: false, Error: "missing state"}
	}
	return Response{OK: true, Message: "verified"}
}

func (s *Server) rollback() Response {
	if s.lastGood == nil {
		return Response{OK: false, Error: "no last known good"}
	}
	if err := s.apply(s.lastGood); err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	return Response{OK: true, Message: "rolled back"}
}

func (s *Server) apply(st *core.NodeDesiredState) error {
	if runtime.GOOS != "linux" {
		log.Printf("netd dry-run apply generation=%d node=%s links=%d (wireguard-go)", st.Generation, st.NodeID, len(st.WireGuardLinks))
		return nil
	}

	// Overlay identity on dummy
	if err := run("ip", "link", "add", st.OverlayIdentity.DummyInterface, "type", "dummy"); err != nil && !strings.Contains(err.Error(), "File exists") {
		// ignore exists
	}
	_ = run("ip", "addr", "replace", st.OverlayIdentity.IPv4+"/32", "dev", st.OverlayIdentity.DummyInterface)
	_ = run("ip", "link", "set", st.OverlayIdentity.DummyInterface, "up")

	if err := s.wg.reconcile(st.WireGuardLinks); err != nil {
		return err
	}

	for _, l := range st.WireGuardLinks {
		if l.InterfaceName == "" {
			continue
		}
		_ = run("ip", "link", "set", l.InterfaceName, "up")
		// Host routes for every AllowedIP (peer + transit overlay destinations).
		ips := l.AllowedIPs
		if len(ips) == 0 && l.PeerOverlayIP != "" {
			ips = []string{l.PeerOverlayIP + "/32"}
		}
		for _, dest := range ips {
			dest = strings.TrimSpace(dest)
			if dest == "" {
				continue
			}
			if !strings.Contains(dest, "/") {
				dest += "/32"
			}
			args := []string{"route", "replace", dest, "dev", l.InterfaceName}
			if st.OverlayIdentity.IPv4 != "" {
				args = append(args, "src", st.OverlayIdentity.IPv4)
			}
			if err := run("ip", args...); err != nil {
				log.Printf("netd: route %s via %s: %v", dest, l.InterfaceName, err)
			}
		}
	}

	if st.ForwardingSettings.IPv4Forwarding {
		_ = run("sysctl", "-w", "net.ipv4.ip_forward=1")
	}

	for _, r := range st.PolicyRules {
		_ = run("ip", "rule", "add", "fwmark", fmt.Sprintf("%d", r.Fwmark), "table", fmt.Sprintf("%d", r.TableID), "priority", fmt.Sprintf("%d", r.Priority+100))
	}
	for _, t := range st.RouteTables {
		if t.TableID == 0 {
			// Main-table overlay routes already installed from AllowedIPs above.
			continue
		}
		for _, rt := range t.Routes {
			args := []string{"route", "replace", rt.Destination, "table", fmt.Sprintf("%d", t.TableID)}
			if rt.Dev != "" {
				args = append(args, "dev", rt.Dev)
			}
			_ = run("ip", args...)
		}
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %v (%s)", name, args, err, string(out))
	}
	return nil
}

// Client talks to netd.
type Client struct {
	Addr string // unix path or host:port
}

func (c *Client) Call(req Request) (*Response, error) {
	addr := c.Addr
	network := "unix"
	if addr == "" {
		addr = DefaultSocket
	}
	if strings.Contains(addr, ":") && !strings.HasPrefix(addr, "/") {
		network = "tcp"
	}
	conn, err := net.Dial(network, addr)
	if err != nil {
		conn, err = net.Dial("tcp", "127.0.0.1:18765")
		if err != nil {
			return nil, err
		}
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
