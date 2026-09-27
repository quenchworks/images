#!/usr/bin/env python3
# velero-plugin-for-aws: GitHub releases of vmware-tanzu/velero-plugin-for-aws.
# Latest only: the 1.14 line pairs with the Velero 1.18 we ship.
from _lib import github, report
report("velero-plugin-for-aws", github("vmware-tanzu/velero-plugin-for-aws"), n=1)
