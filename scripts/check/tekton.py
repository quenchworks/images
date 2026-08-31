#!/usr/bin/env python3
# tekton: GitHub releases on tektoncd/PIPELINE (Tekton Pipelines), tags vX.Y.Z.
# NEWEST STABLE ONLY. Upstream backports across many LTS lines at once
# (1.15/1.14/1.12/1.9/1.6/1.3) and publishes them out of order, so rely on vkey
# sorting rather than the list order.
from _lib import github, report
report("tekton", github("tektoncd/pipeline"), n=1)
