#!/usr/bin/env python3
# forgejo: NOT on GitHub -- the source of truth is Codeberg, so the gh helpers do
# not apply; this reads Codeberg`s Gitea-compatible releases API directly.
# Verified against the live endpoint: tag_name values are vX.Y.Z with real
# prerelease/draft flags, and several major lines (16.0, 15.0, 14.0, 11.0) are
# patched in parallel. build.conf ships the newest LINE, so check per line at
# depth 2: that keeps a security patch on our own line (15.0.x) distinct from a
# new major line, which is a window decision rather than a bump.
from _lib import json_get, report_lines
rels = json_get("https://codeberg.org/api/v1/repos/forgejo/forgejo/releases?limit=50")
tags = [r["tag_name"].lstrip("v") for r in rels
        if not r.get("prerelease") and not r.get("draft")]
report_lines("forgejo", tags, 2)
