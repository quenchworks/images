#!/usr/bin/env python3
# flux-image-reflector-controller: GitHub releases (fluxcd/image-reflector-controller), tag vX.Y.Z. Latest only: Flux ships one version
# of each controller per Flux release.
from _lib import github, report
report("flux-image-reflector-controller", github("fluxcd/image-reflector-controller"), n=1)
