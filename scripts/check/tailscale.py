#!/usr/bin/env python3
# tailscale: GitHub releases, tag vX.Y.Z; stable lines are the even minors. One entry per line.
import re
from _lib import github, report_lines
report_lines("tailscale", github("tailscale/tailscale"), 2, keep=lambda t: int(re.findall(r"\d+", t)[1]) % 2 == 0)
