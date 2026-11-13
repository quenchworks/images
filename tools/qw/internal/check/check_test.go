package check

import "testing"

func TestFilters(t *testing.T) {
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
