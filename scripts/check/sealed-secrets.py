#!/usr/bin/env python3
# sealed-secrets: GitHub releases, tag vX.Y.Z. The repo ALSO cuts helm-vX.Y.Z tags for
# its own Helm chart -- those are not controller releases, so drop them. Window = newest
# patch of the last 3 minor lines (0.36/0.37/0.38), so check per line at depth 2.
from _lib import github, report_lines
report_lines("sealed-secrets", github("bitnami-labs/sealed-secrets"), 2,
             keep=lambda t: not t.startswith("helm-"))
