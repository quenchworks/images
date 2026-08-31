#!/usr/bin/env python3
# livekit: GitHub releases (livekit/livekit), tags vX.Y.Z. build.conf ships the
# newest patch of the newest line, so n=1.
from _lib import github, report
report("livekit", github("livekit/livekit"), n=1)
