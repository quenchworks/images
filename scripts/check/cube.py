#!/usr/bin/env python3
# cube: GitHub releases of cube-js/cube (vX.Y.Z). Held (see apps/cube/build.conf); a new
# release is the cue to re-measure the native addon's crates.
from _lib import github, report_lines
report_lines("cube", github("cube-js/cube"), 1)
