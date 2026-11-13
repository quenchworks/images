package main

import "testing"

func TestOrdering(t *testing.T) {
	for _, c := range [][2]string{{"1.9.9", "1.10.0"}, {"17.2", "REL_17_3"}, {"2.4", "2.4.1"}} {
		if !vless(c[0], c[1]) || vless(c[1], c[0]) {
			t.Errorf("want %s < %s", c[0], c[1])
		}
	}
	if lineKey("26.7.14.3", 2) != "26.7" {
		t.Error("lineKey")
	}
	if cmpKey(apkKey("1.2.3-r2"), apkKey("1.2.3-r10")) >= 0 || cmpKey(apkKey("1.2.10-r0"), apkKey("1.2.3-r9")) <= 0 {
		t.Error("apkKey orders revisions and versions")
	}
	if !numDots("2.10.3") || numDots("2.10.3rc1") || numDots("...") {
		t.Error("numDots")
	}
	if !full(`\d+\.\d+\.\d+`)("1.2.3") || full(`\d+\.\d+\.\d+`)("1.2.3-rc1") {
		t.Error("full anchors both ends")
	}
	f := group(`selenium-(\d+\.\d+\.\d+)`)
	if got := f.apply([]string{"selenium-4.52.0", "4.52.0", "selenium-4.52.0-rc1"}); len(got) != 1 || got[0] != "4.52.0" {
		t.Errorf("group: %v", got)
	}
}

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
