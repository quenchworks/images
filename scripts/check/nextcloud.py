#!/usr/bin/env python3
# nextcloud: official release tarball nextcloud-<ver>.tar.bz2 from
# download.nextcloud.com; versions tracked via GitHub releases (nextcloud/server,
# tag vX.Y.Z, rc/beta are marked prerelease and dropped). We ship the newest patch
# of the last 3 stable MAJOR lines (32, 33, 34) -> per-line check at depth=1 (major).
from _lib import github, report_lines
report_lines("nextcloud", github("nextcloud/server"), 1)
