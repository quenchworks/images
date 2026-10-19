#!/usr/bin/env python3
# rustfs: stable since 1.0.0 (2026-10). Track plain X.Y.Z releases only: the tags
# also carry -alpha.N, -beta.N and -rc.N, which vkey cannot order against stable.
#
# Uses github_asset_releases(), NOT github_tags(): upstream cuts preview versions as
# git TAGS ONLY, with no release and therefore no downloadable zip. A tag-based check
# proposed 1.0.0-beta.12-preview.1, whose asset 404s -> the build failed with curl
# exit 22. Only versions that actually ship an asset are installable.
from _lib import github_asset_releases, report
report("rustfs", github_asset_releases("rustfs/rustfs"),
       keep=lambda v: all(p.isdigit() for p in v.split(".")))
