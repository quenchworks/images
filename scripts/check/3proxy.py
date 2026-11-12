#!/usr/bin/env python3
# 3proxy: GitHub releases, tag X.Y.Z (no v). Newest stable only; the 0.9.9.x maintenance
# line and -preN tags are not shipped.
import re
from _lib import github, report
report("3proxy", [v for v in github("3proxy/3proxy") if re.fullmatch(r"[1-9]\d*\.\d+\.\d+", v)], n=1)
