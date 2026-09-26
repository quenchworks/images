#!/usr/bin/env python3
# flux-kustomize-controller: GitHub releases (fluxcd/kustomize-controller), tag vX.Y.Z. Latest only: Flux ships one version
# of each controller per Flux release.
from _lib import github, report
report("flux-kustomize-controller", github("fluxcd/kustomize-controller"), n=1)
