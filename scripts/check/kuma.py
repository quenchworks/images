#!/usr/bin/env python3
# kuma: GitHub releases (kumahq/kuma). NEWEST-ONLY. Upstream backports across
# many minor lines at once (2.14/2.13/2.12/2.11/...) and is inconsistent about
# the "v" prefix (2.9.19 has none); github() lstrips it either way.
from _lib import github, report
report("kuma", github("kumahq/kuma"), n=1)
