#!/usr/bin/env python3
# drupal: official Drupal core release tarballs, enumerated from drupal.org's
# release-history XML feed (updates.drupal.org). We ship the newest patch of the
# last 3 stable MINOR lines (all 11.x right now), so a per-line (depth=2) check
# flags a newer patch on any line we ship and announces a brand-new minor line
# beyond our newest. Stable only: the pattern captures X.Y.Z, so the x-dev /
# alpha / beta / rc entries in the feed are ignored.
from _lib import scrape, report_lines

vers = scrape(
    "https://updates.drupal.org/release-history/drupal/current",
    r"<version>(\d+\.\d+\.\d+)</version>",
)
report_lines("drupal", vers, 2)
