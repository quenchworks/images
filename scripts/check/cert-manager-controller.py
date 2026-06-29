#!/usr/bin/env python3
# cert-manager-controller: GitHub releases (cert-manager/cert-manager). All four
# cert-manager component images track the same upstream release.
from _lib import github, report
report("cert-manager-controller", github("cert-manager/cert-manager"))
