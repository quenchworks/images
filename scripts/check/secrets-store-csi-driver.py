#!/usr/bin/env python3
# secrets-store-csi-driver: GitHub releases, tag vX.Y.Z. Window = latest patch of the last 2 minor lines.
from _lib import github, report
report("secrets-store-csi-driver", github("kubernetes-sigs/secrets-store-csi-driver"), n=2)
