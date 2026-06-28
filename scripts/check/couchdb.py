#!/usr/bin/env python3
# couchdb: apache dist; versions = GitHub tags X.Y.Z (apache/couchdb).
import re
from _lib import github_tags, report
report("couchdb", github_tags("apache/couchdb"), keep=lambda t: bool(re.fullmatch(r"\d+\.\d+\.\d+", t)))
