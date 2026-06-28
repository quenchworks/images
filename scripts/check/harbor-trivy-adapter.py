#!/usr/bin/env python3
# harbor-trivy-adapter: image version tracks the HARBOR release (goharbor/harbor);
# each Harbor release pins its bundled harbor-scanner-trivy.
from _lib import github, report
report("harbor-trivy-adapter", github("goharbor/harbor"))
