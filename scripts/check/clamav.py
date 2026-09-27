#!/usr/bin/env python3
# clamav: GitHub releases of Cisco-Talos/clamav, tagged clamav-X.Y.Z. Two lines
# (the feature line and the LTS line), checked per line against build.conf.
from _lib import github, report_lines

tags = [t[len("clamav-"):] for t in github("Cisco-Talos/clamav") if t.startswith("clamav-")]
report_lines("clamav", tags, 2)
