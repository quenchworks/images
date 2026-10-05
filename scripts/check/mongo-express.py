#!/usr/bin/env python3
# mongo-express: npm's "latest" dist-tag, newest-only. Upstream ships the 1.1.0 rc train as
# latest, and vkey ranks 1.1.0-rc-4 above a later 1.1.0, so read the tag npm points at
# instead of sorting every version. When 1.1.0 ships this reports have > upstream: bump.
from _lib import json_get, report
report("mongo-express", [json_get("https://registry.npmjs.org/mongo-express")["dist-tags"]["latest"]], n=1)
