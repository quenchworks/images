#!/usr/bin/env python3
# nginx: build fetches nginx.org; versions = GitHub tags release-X.Y.Z (nginx/nginx).
# Track the STABLE lines only (even minor: 1.26/1.28/1.30). Odd minors are the
# mainline/dev branch, deliberately NOT shipped, so filter them out here.
import re
from _lib import github_tags, report


def _stable(t):
    m = re.fullmatch(r"release-(\d+)\.(\d+)\.(\d+)", t)
    return bool(m) and int(m.group(2)) % 2 == 0


report("nginx", github_tags("nginx/nginx"),
       keep=_stable, clean=lambda t: t.replace("release-", ""))
