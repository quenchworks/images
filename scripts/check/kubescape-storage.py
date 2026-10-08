#!/usr/bin/env python3
# kubescape-storage: GitHub releases of kubescape/storage, tag vX.Y.Z. Continuous v0.x releases; ship the newest.
from _lib import github, report
report("kubescape-storage", github("kubescape/storage"), n=1)
