#!/usr/bin/env python3
# backing-image-manager: versions follow the Longhorn release (longhorn/longhorn tags).
from _lib import github, report_lines
report_lines("backing-image-manager", github("longhorn/longhorn"), 3)
