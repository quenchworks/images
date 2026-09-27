#!/usr/bin/env python3
# percona-xtradb-cluster-operator: GitHub releases of
# percona/percona-xtradb-cluster-operator, newest patch of each line we ship
# (1.20, 1.19, 1.18).
from _lib import github, report_lines
report_lines("percona-xtradb-cluster-operator", github("percona/percona-xtradb-cluster-operator"), depth=2)
