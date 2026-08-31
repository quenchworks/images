#!/usr/bin/env python3
# argo-workflows: GitHub releases (argoproj/argo-workflows), tags vX.Y.Z. The
# recipe tracks a MINOR LINE (build.conf: "latest patch of the last 2 minor
# lines"), and upstream really does patch several lines in parallel -- verified
# against the releases API, not assumed: 3.7.18, 4.0.10 and 4.1.2 are all live.
# So check per line at depth 2, which reports the patch on OUR line separately
# from a brand-new upstream line (a window decision, not a bump).
from _lib import github, report_lines
report_lines("argo-workflows", github("argoproj/argo-workflows"), 2)
