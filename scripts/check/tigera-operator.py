#!/usr/bin/env python3
# tigera-operator: pinned to the release that deploys the Calico version our calico-*
# images are built from (see build.conf), so report the newest upstream release for
# awareness; move it only together with the calico images.
from _lib import github, report

report("tigera-operator", github("tigera/operator"), n=1)
