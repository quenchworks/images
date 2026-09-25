#!/usr/bin/env python3
# trivy-operator: GitHub releases of aquasecurity/trivy-operator (stable vX.Y.Z), newest-only.
import re
from _lib import github, report
report("trivy-operator", [v for v in github("aquasecurity/trivy-operator") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
