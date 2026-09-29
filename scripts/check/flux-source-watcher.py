#!/usr/bin/env python3
# flux-source-watcher: GitHub releases (fluxcd/source-watcher), tag vX.Y.Z. Latest only: Flux ships one version
# of each controller per Flux release.
from _lib import github, report
report("flux-source-watcher", github("fluxcd/source-watcher"), n=1)
