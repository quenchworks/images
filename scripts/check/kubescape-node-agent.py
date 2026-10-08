#!/usr/bin/env python3
# kubescape-node-agent: GitHub releases of kubescape/node-agent, tag vX.Y.Z. Continuous v0.x releases; ship the newest.
from _lib import github, report
report("kubescape-node-agent", github("kubescape/node-agent"), n=1)
