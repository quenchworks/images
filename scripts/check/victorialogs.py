#!/usr/bin/env python3
# victorialogs: GitHub releases on VictoriaMetrics/VICTORIALOGS -- VictoriaLogs
# now ships from its own repo, NOT out of VictoriaMetrics/VictoriaMetrics (see
# apps/victorialogs/build.conf). Tags are vX.Y.Z, published slightly out of
# order (v1.51.1 after v1.52.0), so rely on vkey sorting. NEWEST-ONLY.
# NOTE: a bump also needs the tag`s dereferenced COMMIT for build.conf`s COMMIT
# map -- this checker reports the version only.
from _lib import github, report
report("victorialogs", github("VictoriaMetrics/VictoriaLogs"), n=1)
