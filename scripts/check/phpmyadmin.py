#!/usr/bin/env python3
# phpmyadmin: upstream publishes the release tarball on files.phpmyadmin.net and advertises
# the current stable version in its own version JSON (the same feed phpMyAdmin's in-app
# version check uses). "version" is the ONE supported line -- the "releases" array also
# carries the frozen legacy 4.9.11 (php >=5.5,<8.0), which cannot run on our php-8.3
# runtime -- so we track a single version (n=1), matching build.conf's window.
from _lib import json_get, report
d = json_get("https://www.phpmyadmin.net/home_page/version.json")
report("phpmyadmin", [d["version"]], n=1)
