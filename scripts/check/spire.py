#!/usr/bin/env python3
# spire: GitHub releases (spiffe/spire), tag vX.Y.Z. LATEST LINE ONLY (see
# apps/spire/build.conf: 1.13/1.14 carry an unfixable docker/docker CVE).
from _lib import github, report
report("spire", github("spiffe/spire"), n=1)
