#!/usr/bin/env python3
# harbor-registry: image version tracks the HARBOR release (goharbor/harbor); the
# registry is goharbor/distribution bundled per Harbor release.
from _lib import github, report
report("harbor-registry", github("goharbor/harbor"))
