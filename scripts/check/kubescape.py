#!/usr/bin/env python3
# kubescape: GitHub releases, tag vX.Y.Z. Upstream maintains one line; ship the newest patch.
from _lib import github, report
report("kubescape", github("kubescape/kubescape"), n=1)
