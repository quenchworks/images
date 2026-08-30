#!/usr/bin/env python3
# istiod: GitHub releases on istio/istio, bare tags (no "v" prefix, github()'s
# lstrip("v") is a no-op here). Upstream patches three minor lines in parallel
# (1.28/1.29/1.30), so window = newest patch of the last 3 minor lines -> check
# per line at depth 2, matching the recipe's VERSIONS.
from _lib import github, report_lines
report_lines("istiod", github("istio/istio"), 2)
