#!/usr/bin/env python3
# postgres-documentdb: same DocumentDB extension (microsoft/documentdb) packaged on
# postgresql; image version = extension version, tags vX.Y-Z.
from _lib import github_tags, report
report("postgres-documentdb", github_tags("microsoft/documentdb"), clean=lambda t: t.replace("-", "."))
