#!/usr/bin/env python3
# quickwit: GitHub releases (quickwit-oss/quickwit), tags vX.Y.Z. NEWEST-ONLY.
# The repo also publishes non-version releases in the same namespace (bare commit
# shas, "lambda-<sha>", "aws-lambda-beta-NN"); report() drops any candidate that
# does not start with a digit, so those are filtered without an explicit keep.
from _lib import github, report
report("quickwit", github("quickwit-oss/quickwit"), n=1)
