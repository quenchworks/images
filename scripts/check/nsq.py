#!/usr/bin/env python3
# nsq: GitHub releases (nsqio/nsq), tags vX.Y.Z. build.conf ships the last three
# releases (1.2.0, 1.2.1, 1.3.0) as a flat window, not per minor line, so a flat
# newest-3 is the matching check. The ancient v1.0.0-compat tag sorts below them.
from _lib import github, report
report("nsq", github("nsqio/nsq"), n=3)
