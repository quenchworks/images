#!/usr/bin/env python3
# csi-snapshotter: GitHub releases (kubernetes-csi/external-snapshotter).
from _lib import github, report_lines
report_lines("csi-snapshotter", github("kubernetes-csi/external-snapshotter"), 3)
