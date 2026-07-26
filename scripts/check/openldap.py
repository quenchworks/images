#!/usr/bin/env python3
# openldap: Wolfi versioned apk openldap-<line> (FROM_SOURCE=0). Upstream keeps a
# single stable line (2.6.x), so depth=2 compares our 2.6 entry against Wolfi's
# newest openldap-2.6 patch, and flags a brand-new line (2.7) if it ever appears.
# `wolfi_majors` needs a numeric-suffix prefix, which "openldap-" gives us
# (openldap-2.6); the unversioned `openldap` apk is stale and deliberately ignored.
from _lib import report_wolfi_lines
report_wolfi_lines("openldap", "openldap-", 2)
