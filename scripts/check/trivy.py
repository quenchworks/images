#!/usr/bin/env python3
# trivy: GitHub releases (aquasecurity/trivy).
from _lib import github, report
report("trivy", github("aquasecurity/trivy"))
