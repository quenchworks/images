#!/usr/bin/env python3
# kubescape-synchronizer: GitHub releases of kubescape/synchronizer, tag vX.Y.Z. Continuous v0.x releases; ship the newest.
from _lib import github, report
report("kubescape-synchronizer", github("kubescape/synchronizer"), n=1)
