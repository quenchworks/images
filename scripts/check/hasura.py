#!/usr/bin/env python3
# hasura: GitHub releases of hasura/graphql-engine (the server ships in the
# -ce Docker tags of the same version). Latest patch of the last 3 minor lines.
from _lib import github, report_lines
report_lines("hasura", [v for v in github("hasura/graphql-engine") if v.startswith("2.")], depth=2)
