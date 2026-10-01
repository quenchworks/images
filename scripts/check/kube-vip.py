#!/usr/bin/env python3
# kube-vip: GitHub releases (kube-vip/kube-vip), newest patch per minor line.
from _lib import github, report_lines
report_lines("kube-vip", github("kube-vip/kube-vip"), 2)
