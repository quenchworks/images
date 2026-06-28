#!/usr/bin/env python3
# nginx: build fetches nginx.org; versions = GitHub tags release-X.Y.Z (nginx/nginx).
import re
from _lib import github_tags, report
report("nginx", github_tags("nginx/nginx"),
       keep=lambda t: t.startswith("release-"), clean=lambda t: t.replace("release-", ""))
