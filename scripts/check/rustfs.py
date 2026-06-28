#!/usr/bin/env python3
# rustfs: pre-1.0 BETA project. Tags mix 1.0.0-alpha.N and 1.0.0-beta.N; we track
# the beta line. Keep only beta tags (else alpha.99 sorts above beta.8, since the
# numeric vkey can't tell alpha<beta). build.conf uses 1.0.0_beta8; digits match.
from _lib import github_tags, report
report("rustfs", github_tags("rustfs/rustfs"), keep=lambda v: "beta" in v)
