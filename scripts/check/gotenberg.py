#!/usr/bin/env python3
# gotenberg: GitHub releases (gotenberg/gotenberg), tag vX.Y.Z. LATEST ONLY (see
# apps/gotenberg/build.conf).
from _lib import github, report
report("gotenberg", github("gotenberg/gotenberg"), n=1)
