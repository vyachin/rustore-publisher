package version_test

import (
	"strings"
	"testing"

	"github.com/vyachin/rustore-publisher/internal/version"
)

func TestNewReturnsBuildInfo(t *testing.T) {
	original := version.New()
	t.Cleanup(func() {
		version.Version, version.Commit, version.Date = original.Version, original.Commit, original.Date
	})

	version.Version = "0.0.1"
	version.Commit = "abc1234"
	version.Date = "2026-11-09T18:00:00Z"

	info := version.New()
	if info.Version != "0.0.1" || info.Commit != "abc1234" || info.Date != "2026-11-09T18:00:00Z" {
		t.Fatalf("New() = %+v", info)
	}

	text := info.String()
	for _, want := range []string{"0.0.1", "abc1234", "2026-11-09T18:00:00Z"} {
		if !strings.Contains(text, want) {
			t.Errorf("String() = %q, want %q", text, want)
		}
	}
}
