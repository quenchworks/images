#!/usr/bin/env python3
# argocd: GitHub releases, tag vX.Y.Z. Upstream keeps three minor lines patched in
# parallel (3.4/3.3/3.2), so a flat "newest 3" would collapse onto the newest line.
# Window = newest patch of the last 3 minor lines -> check per line at depth 2.
# Release candidates are already excluded by github()'s prerelease filter.
from _lib import github, report_lines
report_lines("argocd", github("argoproj/argo-cd"), 2)
