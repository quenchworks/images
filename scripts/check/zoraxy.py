#!/usr/bin/env python3
# zoraxy: GitHub releases, tag vX.Y.Z; -rcN tags are skipped by the semver filter. Latest only.
from _lib import github, report
report("zoraxy", github("tobychui/zoraxy"), n=1)
