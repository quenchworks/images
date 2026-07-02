#!/usr/bin/env python3
# graylog: no GitHub Releases -- only git tags. The official tarball at
# packages.graylog2.org/releases/graylog/graylog-<tag>.tgz mirrors each stable
# tag, so the tags are the discovery source (keep only pure X.Y.Z stable tags,
# dropping -alpha/-beta/-rc/-preview). We ship the newest patch of the last 3
# minor lines -> per-line check at depth 2.
import re
from _lib import github_tags, report_lines

STABLE = re.compile(r"\d+\.\d+\.\d+$")
report_lines(
    "graylog",
    github_tags("Graylog2/graylog2-server"),
    2,
    keep=lambda t: bool(STABLE.fullmatch(t)),
)
