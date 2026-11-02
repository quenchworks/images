#!/usr/bin/env python3
# teleport: GitHub releases v<major>.<minor>.<patch>; upstream patches three majors at once
# (16/17/18), and the recipe ships the newest release only.
from _lib import github, report
report("teleport", github("gravitational/teleport"), n=1)
