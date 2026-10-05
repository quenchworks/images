#!/usr/bin/env python3
# versitygw: GitHub releases, tag vX.Y.Z. Window = latest patch of last 3 minor lines.
from _lib import github, report
report("versitygw", github("versity/versitygw"))
