#!/usr/bin/env python3
# velero: GitHub releases (vmware-tanzu/velero), tags vX.Y.Z. NEWEST-ONLY.
from _lib import github, report
report("velero", github("vmware-tanzu/velero"), n=1)
