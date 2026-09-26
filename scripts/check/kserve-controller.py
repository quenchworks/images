#!/usr/bin/env python3
# kserve-controller: GitHub releases of kserve/kserve, newest only (older lines
# need keda and opentelemetry-operator jumps; see apps/kserve-controller/build.conf).
from _lib import github, report
report("kserve-controller", github("kserve/kserve"), n=1)
