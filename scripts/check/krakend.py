#!/usr/bin/env python3
# krakend (KrakenD Community Edition): GitHub releases on krakend/krakend-ce,
# tag vX.Y.Z. _lib.github() already drops drafts and prereleases; upstream cuts
# no -rc/-beta tags on this repo, so no extra keep= filter is needed.
#
# n=1 on purpose, and it is upstream's own policy, not a toolchain call:
# SECURITY.md says CE "only fixes the latest version of the software, and does
# not patch prior versions", so older minor lines are dead ends -- tracking them
# would ship a line that will never get a security fix. See build.conf.
from _lib import github, report

report("krakend", github("krakend/krakend-ce"), n=1)
