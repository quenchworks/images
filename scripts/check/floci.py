#!/usr/bin/env python3
# floci: built from the GitHub source tag (no release jar/binary; tags have no
# leading `v`). Single fast-moving line (1.5.x); we ship the newest 3 patches.
from _lib import github, report
report("floci", github("floci-io/floci"), n=3)
