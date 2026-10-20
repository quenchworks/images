#!/usr/bin/env python3
# documentdb: the Microsoft DocumentDB Postgres extension (microsoft/documentdb),
# tags vX.Y-Z (e.g. v0.112-0). Our image version is the extension version.
# Only X.Y-Z tags count: v1.0-RC1/RC2 are pre-releases older than v1.0-0, and
# "1.0.RC2" used to sort above 1.0.0 (2026-10-20).
import re
from _lib import github_tags, report
report("documentdb", github_tags("microsoft/documentdb"),
       keep=lambda t: re.fullmatch(r"\d+\.\d+-\d+", t), clean=lambda t: t.replace("-", "."))
