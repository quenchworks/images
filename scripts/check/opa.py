#!/usr/bin/env python3
# opa: GitHub releases, tag vX.Y.Z. Window = latest patch of last 3 minor lines.
from _lib import github, report
report("opa", github("open-policy-agent/opa"))
