#!/usr/bin/env python3
# linkerd-cni: GitHub releases of linkerd/linkerd2-proxy-init tagged cni-plugin/vX.Y.Z (the repo also
# tags proxy-init/ and validator/). Ship the newest.
from _lib import github, report
report("linkerd-cni", github("linkerd/linkerd2-proxy-init"), n=1,
       keep=lambda t: t.startswith("cni-plugin/v"), clean=lambda t: t[len("cni-plugin/v"):])
