#!/usr/bin/env python3
# ingress-nginx: Wolfi versioned apk ingress-nginx-controller-<minor>, PLUS a melange
# shim that rebuilds the controller binary from source (FROM_SOURCE=1 since 2026-08-27;
# Wolfi's apk is built against vulnerable x/net + x/text). See apps/ingress-nginx/build.conf.
from _lib import report_wolfi_lines
report_wolfi_lines("ingress-nginx", "ingress-nginx-controller-", 2)
