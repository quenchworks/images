#!/usr/bin/env python3
# deno: the recipe ships the OFFICIAL upstream release binary (denoland/deno),
# NOT the Wolfi apk (which lags upstream) — track GitHub releases, newest stable
# only (github() already drops prereleases/drafts; keep= guards odd tags).
# FROZEN at 2.9.7 since 2026-10-14 (user decision, see apps/deno/build.conf): the
# runtime is winding down after the Cloudflare deal, so newer tags are filtered out
# and the daily update check never offers a bump. Drop the cap to resume updates.
from _lib import github, report, vkey
FROZEN = "2.9.7"
report("deno", github("denoland/deno"),
       keep=lambda t: all(p.isdigit() for p in t.split(".")) and vkey(t) <= vkey(FROZEN))
