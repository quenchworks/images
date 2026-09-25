#!/usr/bin/env python3
# uptime-kuma: GitHub releases of louislam/uptime-kuma (stable X.Y.Z), newest-only.
import re
from _lib import github, report
report("uptime-kuma", [v for v in github("louislam/uptime-kuma") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
