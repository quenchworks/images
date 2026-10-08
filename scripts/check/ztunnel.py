#!/usr/bin/env python3
# ztunnel: git tags on istio/ztunnel (it cuts no GitHub releases), bare X.Y.Z cut alongside istio/istio, three minor
# lines in parallel, so window = newest patch of the last 3 minor lines, like istiod.
import re
from _lib import github_tags, report_lines
report_lines("ztunnel", [t for t in github_tags("istio/ztunnel") if re.fullmatch(r"\d+\.\d+\.\d+", t)], 2)
