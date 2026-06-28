#!/usr/bin/env python3
# haproxy: haproxy.org; versions = GitHub tags vX.Y.Z (haproxy/haproxy). Drop -dev.
import re
from _lib import github_tags, report
report("haproxy", github_tags("haproxy/haproxy"), keep=lambda t: bool(re.fullmatch(r"\d+\.\d+\.\d+", t)))
