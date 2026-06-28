#!/usr/bin/env python3
# php: tag is full X.Y.Z (tracks Wolfi's php-<minor> resolved patch, like node).
# Per-line check: compare each line's newest Wolfi patch to our pinned full version
# (flags drift -> bump).
from _lib import report_wolfi_lines
report_wolfi_lines("php", "php-", 2)
