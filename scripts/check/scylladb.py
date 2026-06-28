#!/usr/bin/env python3
# scylladb: s3 downloads; versions = GitHub tags scylla-X.Y.Z (scylladb/scylladb).
# Drop -rcN/-candidate prereleases.
import re
from _lib import github_tags, report
report("scylladb", github_tags("scylladb/scylladb"),
       keep=lambda t: bool(re.fullmatch(r"scylla-\d+\.\d+\.\d+", t)),
       clean=lambda t: t.replace("scylla-", ""))
