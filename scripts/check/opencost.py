#!/usr/bin/env python3
# opencost: GitHub releases on opencost/opencost, tags vX.Y.Z.
#
# RELEASES, NOT TAGS. Upstream tags a 2.x line whose every entry is still a
# release candidate (2.6.0-rc.0 and below as of 2026-09-21). Reading tags would
# put those RCs at the top of the window; github() filters on the releases
# endpoint's prerelease flag, which keeps them out.
#
# Window = newest patch of the last three stable minor lines, so depth 2.
from _lib import github, report_lines
report_lines("opencost", github("opencost/opencost"), 2)
