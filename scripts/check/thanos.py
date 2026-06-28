#!/usr/bin/env python3
# thanos: GitHub releases (thanos-io/thanos). (melange refs prometheus/common as a
# build dep — NOT the upstream.)
from _lib import github, report
report("thanos", github("thanos-io/thanos"))
