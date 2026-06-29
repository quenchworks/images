#!/usr/bin/env python3
# kubectl: Wolfi versioned apk kubectl-<minor> (FROM_SOURCE=0). Per-line newest patch.
from _lib import report_wolfi_lines
report_wolfi_lines("kubectl", "kubectl-", 2)
