#!/usr/bin/env python3
# rustfs: pre-1.0 BETA project. Tags mix 1.0.0-alpha.N and 1.0.0-beta.N; we track
# the beta line. Keep only beta tags (else alpha.99 sorts above beta.8, since the
# numeric vkey can't tell alpha<beta). build.conf uses 1.0.0_beta8; digits match.
#
# Uses github_asset_releases(), NOT github_tags(): upstream cuts preview versions as
# git TAGS ONLY, with no release and therefore no downloadable zip. A tag-based check
# proposed 1.0.0-beta.12-preview.1, whose asset 404s -> the build failed with curl
# exit 22. Only versions that actually ship an asset are installable.
from _lib import github_asset_releases, report
report("rustfs", github_asset_releases("rustfs/rustfs"), keep=lambda v: "beta" in v)
