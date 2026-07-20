package netd

import (
	"strings"
	"testing"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/security"
)

func TestBuildUAPIInitiator(t *testing.T) {
	priv, pub, err := security.GenerateWGKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peerPriv, peerPub, err := security.GenerateWGKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_ = peerPriv
	uapi, err := buildUAPI(core.WireGuardLinkCfg{
		LinkID: "l1", InterfaceName: "pwl-test", IsInitiator: true,
		NodePrivateKey: priv, PeerPublicKey: peerPub,
		PeerEndpoint: "1.2.3.4:14303", PeerOverlayIP: "10.250.0.2",
		AllowedIPs: []string{"10.250.0.2/32"},
		PersistentKeepalive: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"private_key=", "public_key=", "endpoint=1.2.3.4:14303", "allowed_ip=10.250.0.2/32", "persistent_keepalive_interval=25", "replace_peers=true"} {
		if !strings.Contains(uapi, want) {
			t.Fatalf("missing %q in:\n%s", want, uapi)
		}
	}
	if strings.Contains(uapi, "listen_port=") {
		t.Fatal("initiator should not set listen_port")
	}
	_ = pub
}

func TestBuildUAPIListener(t *testing.T) {
	priv, _, err := security.GenerateWGKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, peerPub, err := security.GenerateWGKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	uapi, err := buildUAPI(core.WireGuardLinkCfg{
		LinkID: "l2", InterfaceName: "pwl-l2", IsInitiator: false,
		NodePrivateKey: priv, PeerPublicKey: peerPub,
		ListenPort: 14310, PeerOverlayIP: "10.250.0.1",
		PeerEndpoint: "10.0.0.2:14311", PersistentKeepalive: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(uapi, "listen_port=14310") {
		t.Fatalf("want listen_port:\n%s", uapi)
	}
	if !strings.Contains(uapi, "endpoint=10.0.0.2:14311") {
		t.Fatalf("want endpoint:\n%s", uapi)
	}
}

func TestBuildUAPIAllowedIPs(t *testing.T) {
	priv, _, err := security.GenerateWGKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, peerPub, err := security.GenerateWGKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	uapi, err := buildUAPI(core.WireGuardLinkCfg{
		LinkID: "l3", InterfaceName: "pwl-l3", IsInitiator: true,
		NodePrivateKey: priv, PeerPublicKey: peerPub,
		PeerEndpoint: "1.2.3.4:14303", PeerOverlayIP: "10.250.0.2",
		AllowedIPs: []string{"10.250.0.2/32", "10.250.0.3/32"},
		PersistentKeepalive: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(uapi, "allowed_ip=10.250.0.2/32") || !strings.Contains(uapi, "allowed_ip=10.250.0.3/32") {
		t.Fatalf("want both allowed_ips:\n%s", uapi)
	}
}

func TestKeyToHexRejectsBad(t *testing.T) {
	if _, err := keyToHex("not-base64!!"); err == nil {
		t.Fatal("expected error")
	}
}
