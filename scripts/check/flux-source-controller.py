#!/usr/bin/env python3
# flux-source-controller: GitHub releases (fluxcd/source-controller), tag vX.Y.Z. Latest only: Flux ships one version
# of each controller per Flux release.
from _lib import github, report
report("flux-source-controller", github("fluxcd/source-controller"), n=1)
