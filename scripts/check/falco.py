#!/usr/bin/env python3
# falco: GitHub releases of falcosecurity/falco, tagged X.Y.Z (no v). Latest only.
from _lib import github, report
report("falco", github("falcosecurity/falco"), n=1)
