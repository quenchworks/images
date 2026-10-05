#!/usr/bin/env python3
# trust-manager: GitHub releases of cert-manager/trust-manager (stable X.Y.Z; alphas skipped).
import re
from _lib import github, report
report("trust-manager", [v for v in github("cert-manager/trust-manager") if re.fullmatch(r"v?\d+\.\d+\.\d+", v)])
