#!/usr/bin/env python3
# cert-manager-cainjector: GitHub releases (cert-manager/cert-manager). All four
# cert-manager component images track the same upstream release.
from _lib import github, report
report("cert-manager-cainjector", github("cert-manager/cert-manager"))
