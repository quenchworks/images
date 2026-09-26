#!/usr/bin/env python3
# dapr: GitHub releases (dapr/dapr), tag vX.Y.Z. We ship the 1.17 and 1.18 lines (see
# apps/dapr/build.conf); a newer line shows as NEW LINE.
from _lib import github, report_lines
report_lines("dapr", github("dapr/dapr"), 2)
