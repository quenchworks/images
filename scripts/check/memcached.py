#!/usr/bin/env python3
# memcached: memcached.org; versions = GitHub tags X.Y.Z (memcached/memcached).
import re
from _lib import github_tags, report
report("memcached", github_tags("memcached/memcached"), keep=lambda t: bool(re.fullmatch(r"\d+\.\d+\.\d+", t)))
