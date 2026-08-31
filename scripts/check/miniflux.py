#!/usr/bin/env python3
# miniflux: GitHub releases on miniflux/V2 (the v2 module is its own repo; the
# recipe fetches github.com/miniflux/v2). Tags are bare X.Y.Z. NEWEST-ONLY.
from _lib import github, report
report("miniflux", github("miniflux/v2"), n=1)
