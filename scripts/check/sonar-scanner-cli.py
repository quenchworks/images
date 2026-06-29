#!/usr/bin/env python3
# sonar-scanner-cli: flat Wolfi apk 'sonar-scanner-cli' (FROM_SOURCE=0). Latest lines.
from _lib import wolfi, report
report("sonar-scanner-cli", wolfi("sonar-scanner-cli"))
