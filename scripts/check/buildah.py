#!/usr/bin/env python3
# buildah: GitHub releases of containers/buildah, newest patch of each line we
# ship (1.45, 1.44). 1.43 is held; see apps/buildah/build.conf.
from _lib import github, report_lines
report_lines("buildah", github("containers/buildah"), depth=2)
