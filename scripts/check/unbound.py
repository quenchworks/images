#!/usr/bin/env python3
# unbound: GitHub releases of NLnetLabs/unbound (tags release-X.Y.Z), newest-only.
import re
from _lib import github, report
vs = [re.sub(r"^release-", "", v) for v in github("NLnetLabs/unbound")]
report("unbound", [v for v in vs if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
