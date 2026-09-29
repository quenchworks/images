#!/usr/bin/env python3
# flux-image-automation-controller: GitHub releases (fluxcd/image-automation-controller), tag vX.Y.Z. Latest only: Flux ships one version
# of each controller per Flux release.
from _lib import github, report
report("flux-image-automation-controller", github("fluxcd/image-automation-controller"), n=1)
