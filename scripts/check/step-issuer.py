#!/usr/bin/env python3
# step-issuer: GitHub releases of smallstep/step-issuer (stable X.Y.Z; rcs skipped).
import re
from _lib import github, report
report("step-issuer", [v for v in github("smallstep/step-issuer") if re.fullmatch(r"v?\d+\.\d+\.\d+", v)])
