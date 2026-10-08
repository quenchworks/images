#!/usr/bin/env python3
# istio-cni: built from the istio/istio tags istiod ships (bare X.Y.Z, three minor lines in
# parallel), so window = newest patch of the last 3 minor lines, like istiod.
from _lib import github, report_lines
report_lines("istio-cni", github("istio/istio"), 2)
