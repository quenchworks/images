#!/usr/bin/env python3
# gitlab-runner: gitlab.com releases (gitlab-org/gitlab-runner), tag vX.Y.Z.
# Latest patch of the last 3 minor lines.
from _lib import json_get, report
rel = json_get("https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab-runner/releases?per_page=40")
report("gitlab-runner", [r["tag_name"].lstrip("v") for r in rel if not r.get("upcoming_release")])
