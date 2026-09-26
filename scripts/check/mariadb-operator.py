#!/usr/bin/env python3
# mariadb-operator: GitHub releases, CalVer X.Y.Z tags (chart tags such as
# mariadb-operator-26.10.1 are skipped). Newest patch of the last 3 lines.
import re
from _lib import github, report_lines
report_lines("mariadb-operator", [v for v in github("mariadb-operator/mariadb-operator") if re.fullmatch(r"\d+\.\d+\.\d+", v)], depth=2)
