#!/usr/bin/env python3
# tetragon-operator: in lockstep with apps/tetragon (same cilium/tetragon release).
from _lib import github, report
report("tetragon-operator", github("cilium/tetragon"), n=1)
