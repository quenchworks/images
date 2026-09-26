#!/usr/bin/env python3
# podman: GitHub releases of containers/podman, newest patch of each line we
# ship (6.1, 6.0). 5.8 is held; see apps/podman/build.conf.
from _lib import github, report_lines
report_lines("podman", github("containers/podman"), depth=2)
