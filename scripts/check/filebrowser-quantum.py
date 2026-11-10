#!/usr/bin/env python3
# filebrowser-quantum: GitHub releases (gtsteffaniak/filebrowser). Stable releases are tagged
# vX.Y.Z-stable and betas vX.Y.Z-beta (both prerelease=false), so keep only -stable and strip it.
from _lib import github, report
report("filebrowser-quantum", [t.removesuffix("-stable") for t in github("gtsteffaniak/filebrowser") if t.endswith("-stable")], n=1)
