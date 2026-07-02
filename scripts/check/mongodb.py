#!/usr/bin/env python3
# mongodb: build fetches fastdl.mongodb.org; versions = GitHub tags rX.Y.Z
# (mongodb/mongo), newest-first. Track the PRODUCTION major lines only (X.0.Z:
# 6.0/7.0/8.0); the X.1/X.2/X.3 "rapid releases" are interim/dev, not shipped, so
# filter them out (also drops prerelease tags like r9.0.0-alpha0).
import re
from _lib import github_tags, report
report("mongodb", github_tags("mongodb/mongo"),
       keep=lambda t: bool(re.fullmatch(r"r\d+\.0\.\d+", t)), clean=lambda t: t[1:])
