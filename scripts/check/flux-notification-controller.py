#!/usr/bin/env python3
# flux-notification-controller: GitHub releases (fluxcd/notification-controller), tag vX.Y.Z. Latest only: Flux ships one version
# of each controller per Flux release.
from _lib import github, report
report("flux-notification-controller", github("fluxcd/notification-controller"), n=1)
