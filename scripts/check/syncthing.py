#!/usr/bin/env python3
# syncthing: GitHub releases on syncthing/syncthing, tags vX.Y.Z. Newest patch of the 2.1 and 2.0 lines.
from _lib import github, report_lines
report_lines("syncthing", github("syncthing/syncthing"), 2)
