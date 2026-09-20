#!/usr/bin/env python3
# bifrost: GitHub tags on maximhq/bifrost.
#
# MONOREPO, TAGGED PER COMPONENT: transports/vX.Y.Z, framework/vX.Y.Z,
# plugins/<name>/vX.Y.Z. The image builds the HTTP gateway in transports/, so
# only that prefix is a candidate; github() would otherwise mix a plugin's
# version into the window.
#
# Releases are not enough here. github() reads the releases endpoint, which
# interleaves every component and pages out the transports history, so this uses
# tags and filters. Window = newest patch of the last three minor lines, depth 2.
from _lib import github_tags, report_lines

tags = [t.split("/", 1)[1].lstrip("v") for t in github_tags("maximhq/bifrost")
        if t.startswith("transports/v")]
report_lines("bifrost", tags, 2, keep=lambda v: "-" not in v)
