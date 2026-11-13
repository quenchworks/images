package core

import "testing"

func TestOrdering(t *testing.T) {
	for _, c := range [][2]string{{"1.9.9", "1.10.0"}, {"17.2", "REL_17_3"}, {"2.4", "2.4.1"}} {
		if !Vless(c[0], c[1]) || Vless(c[1], c[0]) {
			t.Errorf("want %s < %s", c[0], c[1])
		}
	}
	if LineKey("26.7.14.3", 2) != "26.7" {
		t.Error("lineKey")
	}
	if CmpKey(apkKey("1.2.3-r2"), apkKey("1.2.3-r10")) >= 0 || CmpKey(apkKey("1.2.10-r0"), apkKey("1.2.3-r9")) <= 0 {
		t.Error("apkKey orders revisions and versions")
	}
}
