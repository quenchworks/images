#!/usr/bin/env python3
# csi-provisioner: GitHub releases (kubernetes-csi/external-provisioner).
from _lib import github, report_lines
report_lines("csi-provisioner", github("kubernetes-csi/external-provisioner"), 3)
