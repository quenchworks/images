#!/usr/bin/env python3
# guacd: apache/guacamole-server tags X.Y.Z (no GitHub releases). The tags include
# -RCn candidates, so keep plain X.Y.Z only. One release line, latest only.
import re
from _lib import github_tags, report
report("guacd", [t for t in github_tags("apache/guacamole-server") if re.fullmatch(r"\d+\.\d+\.\d+", t)], n=1)
