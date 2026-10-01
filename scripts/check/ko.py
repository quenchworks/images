#!/usr/bin/env python3
# ko: GitHub releases (ko-build/ko), newest patch per minor line.
from _lib import github, report_lines
report_lines("ko", github("ko-build/ko"), 2)
