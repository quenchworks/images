#!/usr/bin/env python3
# deno: the recipe ships the OFFICIAL upstream release binary (denoland/deno),
# NOT the Wolfi apk (which lags upstream) — track GitHub releases, newest stable
# only (github() already drops prereleases/drafts; keep= guards odd tags).
from _lib import github, report
report("deno", github("denoland/deno"),
       keep=lambda t: all(p.isdigit() for p in t.split(".")))
