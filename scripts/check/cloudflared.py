#!/usr/bin/env python3
# cloudflared: GitHub releases on cloudflare/cloudflared, calendar tags YYYY.M.P. LATEST-ONLY.
from _lib import github, report
report("cloudflared", github("cloudflare/cloudflared"), n=1)
