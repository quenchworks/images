#!/usr/bin/env python3
# support-bundle-kit: GitHub releases (rancher/support-bundle-kit). Releases are all 0.0.x, so
# only the newest is compared; the older VERSIONS entries are Longhorn's per-line pins.
from _lib import github, report
report("support-bundle-kit", github("rancher/support-bundle-kit"), n=1)
