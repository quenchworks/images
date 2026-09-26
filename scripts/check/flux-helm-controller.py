#!/usr/bin/env python3
# flux-helm-controller: GitHub releases (fluxcd/helm-controller), tag vX.Y.Z. Latest only: Flux ships one version
# of each controller per Flux release.
from _lib import github, report
report("flux-helm-controller", github("fluxcd/helm-controller"), n=1)
