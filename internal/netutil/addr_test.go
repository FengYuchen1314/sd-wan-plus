package netutil

import "testing"

func TestIsPublicDialable(t *testing.T) {
	cases := map[string]bool{
		"38.59.224.201":   true,
		"192.168.1.10":    false,
		"10.0.0.5":        false,
		"172.16.3.4":      false,
		"100.64.1.2":      false,
		"127.0.0.1":       false,
		"example.com":     true,
		"host.local":      false,
		"[2001:db8::1]":   true,
	}
	for in, want := range cases {
		if got := IsPublicDialable(in); got != want {
			t.Fatalf("%s: got %v want %v", in, got, want)
		}
	}
}
