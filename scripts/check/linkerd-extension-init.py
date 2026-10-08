#!/usr/bin/env python3
# linkerd-extension-init: GitHub releases of linkerd/linkerd-extension-init, tagged release/vX.Y.Z.
# The viz chart pins one version for every edge line (v0.1.12 on 26.7 to 26.9); ship the newest.
from _lib import github, report
report("linkerd-extension-init", github("linkerd/linkerd-extension-init"), n=1,
       keep=lambda t: t.startswith("release/v"), clean=lambda t: t[len("release/v"):])
