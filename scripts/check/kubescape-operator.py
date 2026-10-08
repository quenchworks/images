#!/usr/bin/env python3
# kubescape-operator: GitHub releases of kubescape/operator, tag vX.Y.Z. Continuous v0.x releases; ship the newest.
from _lib import github, report
report("kubescape-operator", github("kubescape/operator"), n=1)
