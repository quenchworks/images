#!/usr/bin/env python3
# wordpress: WordPress core, released from wordpress.org (tarball wordpress-<ver>.tar.gz).
# The official version list is the stable-check API {version: status}. We ship the
# newest patch of the last 3 minor lines (6.8, 6.9, 7.0); tags have NO 'v' prefix and
# the .0 line is "7.0" (not "7.0.0"). report_lines picks the newest per minor line and
# compares to our current build.conf window.
from _lib import json_get, report_lines
d = json_get("https://api.wordpress.org/core/stable-check/1.0/")
report_lines("wordpress", list(d.keys()), depth=2)
