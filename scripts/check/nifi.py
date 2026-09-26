#!/usr/bin/env python3
# nifi: Apache NiFi releases, rel/nifi-X.Y.Z tags on GitHub. Latest only.
import re
from _lib import github_tags, report
tags = [t.split("rel/nifi-", 1)[1] for t in github_tags("apache/nifi") if t.startswith("rel/nifi-")]
report("nifi", [t for t in tags if re.fullmatch(r"\d+\.\d+\.\d+", t)], n=1)
