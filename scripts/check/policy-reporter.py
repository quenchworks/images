#!/usr/bin/env python3
# policy-reporter: GitHub releases of kyverno/policy-reporter. Its chart releases share the
# list as policy-reporter-X.Y.Z tags; only the app's vX.Y.Z tags count.
import re
from _lib import github, report
report("policy-reporter", [v for v in github("kyverno/policy-reporter") if re.fullmatch(r"v\d+\.\d+\.\d+", v)])
