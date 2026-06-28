#!/usr/bin/env python3
# jre: Wolfi versioned apk openjdk-<line>. Per-line newest-patch check.
from _lib import report_wolfi_lines
report_wolfi_lines("jre", "openjdk-", 1)
