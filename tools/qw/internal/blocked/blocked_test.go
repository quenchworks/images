package blocked

import (
	"testing"

	"github.com/quenchworks/images/tools/qw/internal/core"
)

func TestApkVKey(t *testing.T) {
	if core.CmpKey(apkVKey("1.3.2.1_rc20260601-r0"), apkVKey("1.3.2.1_rc20260601-r0")) != 0 ||
		core.CmpKey(apkVKey("1.3.1-r9"), apkVKey("1.3.2.1_rc20260601-r0")) >= 0 ||
		core.CmpKey(apkVKey("1.3.2.1_rc20260601-r1"), apkVKey("1.3.2.1_rc20260601-r0")) <= 0 {
		t.Error("apkVKey")
	}
}
