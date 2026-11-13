package catalog

import "testing"

func TestVersionTags(t *testing.T) {
	for _, k := range []string{"25.0.3", "1.22.22", "4.16.0-dev", "20260621", "3.9.5"} {
		if !isVersionTag(k) {
			t.Errorf("dropped real version %s", k)
		}
	}
	for _, d := range []string{"latest", "sha256-abc", "scan-amd64", "25.0.3-amd64", "25.0.3-arm64"} {
		if isVersionTag(d) {
			t.Errorf("kept registry artifact %s", d)
		}
	}
	if isFullVersion("1.26") || !isFullVersion("2026.04.0") {
		t.Error("isFullVersion")
	}
}
