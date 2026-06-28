#!/usr/bin/env python3
# zookeeper: apache dist; versions = GitHub tags release-X.Y.Z (apache/zookeeper).
import re
from _lib import github_tags, report
report("zookeeper", github_tags("apache/zookeeper"),
       keep=lambda t: bool(re.fullmatch(r"release-\d+\.\d+\.\d+", t)),
       clean=lambda t: t.replace("release-", ""))
