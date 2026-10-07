#!/usr/bin/env python3
# cephcsi: GitHub releases (ceph/ceph-csi), newest only.
from _lib import github, report
report("cephcsi", github("ceph/ceph-csi"), n=1)
