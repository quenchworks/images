#!/usr/bin/env python3
# frp: GitHub releases on fatedier/frp, tags vX.Y.Z. LATEST-ONLY (the dashboard toolchain
# moves with each release).
from _lib import github, report
report("frp", github("fatedier/frp"), n=1)
