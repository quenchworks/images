#!/usr/bin/env python3
# openssl: Wolfi versioned apk openssl-<minor> (FROM_SOURCE=0). Per-line newest patch.
from _lib import report_wolfi_lines
report_wolfi_lines("openssl", "openssl-", 2)
