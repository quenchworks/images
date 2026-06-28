#!/usr/bin/env python3
# php: Wolfi php-<X.Y>. We pin the MINOR LINE only (build.conf VERSIONS=8.3 8.4 8.5);
# the patch floats with Wolfi, so check at line granularity — only a NEWER line
# (e.g. php-8.6) is an update, not a floating patch.
from _lib import report_wolfi_lines
report_wolfi_lines("php", "php-", 2, line_only=True)
