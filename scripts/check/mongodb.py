#!/usr/bin/env python3
# mongodb: track MongoDB's OWN download feed (downloads.mongodb.org/current.json),
# which is the authoritative list of what is actually downloadable -- and the same
# place the build pulls from (fastdl.mongodb.org). Production lines only.
#
# WHY NOT github_tags("mongodb/mongo"): MongoDB now cuts rapid releases (8.1/8.2/8.3)
# and -alpha tags continuously, while github_tags() reads only the FIRST 40 tags. The
# production X.0.Z tags fell off that first page entirely, so the old
# r\d+\.0\.\d+ filter matched nothing and this app reported latest3=[] indefinitely --
# a silent BROKEN CHECK that looked like "up to date" until _lib.report() learned to
# distinguish the two. Paginating tags would work but is fragile for the same reason:
# it depends on how fast upstream cuts interim tags.
import re

from _lib import json_get, report

feed = json_get("https://downloads.mongodb.org/current.json")
versions = [v["version"] for v in feed.get("versions", [])]

# PRODUCTION releases only (X.0.Z). MongoDB's X.1 / X.2 / X.3 are interim "rapid
# releases" that are not supported for production and are not what we ship.
report("mongodb", versions, keep=lambda v: bool(re.fullmatch(r"\d+\.0\.\d+", v)))
