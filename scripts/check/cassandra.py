#!/usr/bin/env python3
# cassandra: apache dist; versions = GitHub tags cassandra-X.Y.Z (apache/cassandra).
import re
from _lib import github_tags, report
report("cassandra", github_tags("apache/cassandra"),
       keep=lambda t: bool(re.fullmatch(r"cassandra-\d+\.\d+\.\d+", t)),
       clean=lambda t: t.replace("cassandra-", ""))
