#!/usr/bin/env python3
# trivy-scanner: tracks aquasecurity/trivy releases, like apps/trivy (VERSIONS move together).
import re
from _lib import github, report
report("trivy-scanner", [v for v in github("aquasecurity/trivy") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
