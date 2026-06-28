#!/usr/bin/env python3
# mongodb: build fetches fastdl.mongodb.org; versions = GitHub tags rX.Y.Z
# (mongodb/mongo), newest-first. Drop prerelease tags (r9.0.0-alpha0).
import re
from _lib import github_tags, report
report("mongodb", github_tags("mongodb/mongo"),
       keep=lambda t: bool(re.fullmatch(r"r\d+\.\d+\.\d+", t)), clean=lambda t: t[1:])
