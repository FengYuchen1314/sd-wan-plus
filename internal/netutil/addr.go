package netutil

import (
	"net"
	"strings"
)

// IsPublicDialable reports whether addr is suitable as a WireGuard peer endpoint
// that another site can dial (not RFC1918 / loopback / link-local / unspecified).
func IsPublicDialable(addr string) bool {
	host := strings.TrimSpace(addr)
	if host == "" {
		return false
	}
	// strip brackets for IPv6 literals without port
	host = strings.Trim(host, "[]")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// hostname: allow (assume public DNS); private DNS names are rare for our installs
		return !strings.HasSuffix(strings.ToLower(host), ".local")
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		// 10/8, 172.16/12, 192.168/16, 100.64/10 (CGNAT)
		if ip4[0] == 10 {
			return false
		}
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return false
		}
		if ip4[0] == 192 && ip4[1] == 168 {
			return false
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return false
		}
		if ip4[0] == 169 && ip4[1] == 254 {
			return false
		}
		return true
	}
	// IPv6 unique local fc00::/7
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return false
	}
	return true
}

// PreferDialableAddress picks the first publicly dialable address from a list of
// (address, addressType) pairs; addressType "public" is preferred over "lan".
func PreferDialableAddress(addrs []struct{ Address, Type string }) string {
	var fallback string
	for _, a := range addrs {
		if strings.EqualFold(a.Type, "lan") {
			continue
		}
		if IsPublicDialable(a.Address) {
			return a.Address
		}
		if fallback == "" {
			fallback = a.Address
		}
	}
	for _, a := range addrs {
		if IsPublicDialable(a.Address) {
			return a.Address
		}
	}
	return ""
}
