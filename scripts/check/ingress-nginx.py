#!/usr/bin/env python3
# ingress-nginx: Wolfi versioned apk ingress-nginx-controller-<minor> (FROM_SOURCE=0).
from _lib import report_wolfi_lines
report_wolfi_lines("ingress-nginx", "ingress-nginx-controller-", 2)
