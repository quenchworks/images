#!/usr/bin/env python3
# dagger: GitHub releases, tag vX.Y.Z (sdk/* tags are skipped by the semver filter).
# Ship-latest-only: the CLI and engine share a minor and only the newest line is patched.
from _lib import github, report
report("dagger", github("dagger/dagger"), n=1)
