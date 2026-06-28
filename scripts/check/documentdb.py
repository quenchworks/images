#!/usr/bin/env python3
# documentdb: the Microsoft DocumentDB Postgres extension (microsoft/documentdb),
# tags vX.Y-Z (e.g. v0.112-0). Our image version is the extension version.
from _lib import github_tags, report
report("documentdb", github_tags("microsoft/documentdb"), clean=lambda t: t.replace("-", "."))
