#!/usr/bin/env python3
# longhorn-share-manager: versions follow the Longhorn release (longhorn/longhorn tags).
from _lib import github, report_lines
report_lines("longhorn-share-manager", github("longhorn/longhorn"), 3)
