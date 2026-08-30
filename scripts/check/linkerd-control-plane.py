#!/usr/bin/env python3
# linkerd-control-plane: GitHub releases on linkerd/linkerd2. Verified against
# the real releases API (not assumed): every release since stable-2.14.10
# (2024-02-20) is edge-YY.M.P -- Buoyant gates true "stable" builds
# commercially, linkerd/linkerd2 itself only cuts edge. github()'s tag_name is
# used as-is (no "v" prefix to strip); the "edge-" prefix is stripped here so
# depth-2 (year.month) line grouping in report_lines works the same way
# istiod's X.Y line grouping does. Window = newest patch of the last 3
# edge year.month lines, matching the recipe's VERSIONS.
from _lib import github, report_lines
tags = [t[len("edge-"):] for t in github("linkerd/linkerd2") if t.startswith("edge-")]
report_lines("linkerd-control-plane", tags, 2)
