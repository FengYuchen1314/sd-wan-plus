package controller

import (
	"testing"
	"time"
)

func TestPickReleaseVersionPrefersBodyThenSuffixed(t *testing.T) {
	names := []string{
		"pathweaver-2026.07.20-linux-amd64.tar.gz",
		"pathweaver-2026.07.20-1516-linux-amd64.tar.gz",
		"pathweaver-2026.07.20-1517-linux-amd64.tar.gz",
		"pathweaver-linux-amd64.tar.gz",
		"pathweaver-0.1.0-abc1234-linux-amd64.tar.gz",
	}
	got := pickReleaseVersion(names, "amd64", "Version: 2026.07.20-5ea3137\nCommit: 5ea3137", time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC))
	if got != "2026.07.20-5ea3137" {
		t.Fatalf("body Version should win, got %q", got)
	}
	got = pickReleaseVersion(names, "amd64", "Commit: 5ea313796cfc1f4a40bd617270f9ea6fb5e14a88", time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC))
	if got != "2026.07.20-1517" {
		t.Fatalf("want newest timed asset, got %q", got)
	}
}

func TestIsReleaseOutdated(t *testing.T) {
	if !isReleaseOutdated("2026.07.20", "2026.07.20-1517") {
		t.Fatal("bare date should be outdated vs timed")
	}
	if isReleaseOutdated("2026.07.20-1517", "2026.07.20-1517") {
		t.Fatal("same version not outdated")
	}
	if !isReleaseOutdated("2026.07.20-1516", "2026.07.20-1517") {
		t.Fatal("older timed should be outdated")
	}
}
