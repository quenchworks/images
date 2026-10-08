#!/usr/bin/env python3
# kubevuln: GitHub releases of kubescape/kubevuln, tag vX.Y.Z. Continuous v0.x releases; ship the newest.
from _lib import github, report
report("kubevuln", github("kubescape/kubevuln"), n=1)
