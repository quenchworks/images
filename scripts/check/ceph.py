#!/usr/bin/env python3
# ceph: the Wolfi ceph-20 apks (FROM_SOURCE=0), newest only.
from _lib import wolfi, report
report("ceph", wolfi("ceph-20"), n=1)
