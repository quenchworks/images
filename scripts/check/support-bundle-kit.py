#!/usr/bin/env python3
# support-bundle-kit: GitHub releases (rancher/support-bundle-kit). Only the newest
# release is compared. The 0.0.x VERSIONS entries are exact pins from Longhorn's
# deploy/longhorn-images.txt per release line (1.11 -> 0.0.88, 1.12 -> 0.0.92,
# 1.13 -> 0.0.98 on 2026-10-20), not semver lines, so 0.0.x releases are left out:
# otherwise the same-line pass proposes an unpinned 0.0.97 next to them. When a new
# Longhorn line ships, read its longhorn-images.txt and add that pin by hand.
from _lib import github, report
report("support-bundle-kit", [t for t in github("rancher/support-bundle-kit")
                              if not t.startswith("0.0.")], n=1)
