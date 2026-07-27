#!/usr/bin/env python3
# crossplane: GitHub releases, tag vX.Y.Z. Upstream keeps several minor lines alive at
# once (2.3/2.2/2.1 plus a long-lived 1.20 maintenance line), so a flat "newest 3" would
# collapse onto one line. Window = newest patch of the last 3 minor lines -> check per
# line at depth 2. Release candidates are already excluded by github()'s prerelease
# filter.
from _lib import github, report_lines
report_lines("crossplane", github("crossplane/crossplane"), 2)
