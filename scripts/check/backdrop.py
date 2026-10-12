#!/usr/bin/env python3
# backdrop: GitHub releases of backdrop/backdrop, where the recipe fetches backdrop.zip.
# Backdrop patches the current and the previous minor, so report_lines compares per X.Y line.
from _lib import github, report_lines
report_lines("backdrop", github("backdrop/backdrop"), depth=2)
