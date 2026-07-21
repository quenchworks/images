#!/usr/bin/env python3
# code-server: GitHub releases, tag vX.Y.Z. Window = latest 3 stable lines.
from _lib import github, report
report("code-server", github("coder/code-server"), n=3)
