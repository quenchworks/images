#!/usr/bin/env python3
# guacamole: apache/guacamole-client tags X.Y.Z (RC tags filtered). One release
# line, latest only. Released together with guacd.
import re
from _lib import github_tags, report
report("guacamole", [t for t in github_tags("apache/guacamole-client") if re.fullmatch(r"\d+\.\d+\.\d+", t)], n=1)
