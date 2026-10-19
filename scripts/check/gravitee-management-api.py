#!/usr/bin/env python3
# gravitee-management-api: Gravitee APIM releases. We build from graviteeio-full-<v>.zip on download.gravitee.io;
# the upstream gateway image tags on Docker Hub track those releases. Per-line, per build.conf.
# An UPDATE is only buildable once the full zip for it answers (it can lag the image).
import re
from _lib import json_get, report_lines
tags = [t["name"] for t in json_get("https://hub.docker.com/v2/repositories/graviteeio/apim-gateway/tags?page_size=100")["results"]]
report_lines("gravitee-management-api", tags, 2, keep=lambda t: re.fullmatch(r"\d+\.\d+\.\d+", t))
