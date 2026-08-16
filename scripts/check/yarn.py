#!/usr/bin/env python3
# yarn: we ship BERRY (yarn 4.x), which lives in yarnpkg/berry and tags its releases
# as "@yarnpkg/cli/X.Y.Z".
#
# The old checker read yarnpkg/yarn -- that is yarn 1.x CLASSIC (frozen at 1.22.x).
# It therefore reported upstream's newest as 1.22.22 while we ship 4.x, i.e. we looked
# permanently "ahead of upstream" and no real 4.x update was ever surfaced. That is the
# have > newest-upstream case _lib.report() now flags as a BROKEN CHECK.
#
# The repo also tags other workspace packages (@yarnpkg/plugin-*, @yarnpkg/sdks, ...),
# so filter to the CLI package before taking the version off the end of the tag.
from _lib import github, report

report(
    "yarn",
    github("yarnpkg/berry"),
    keep=lambda t: t.startswith("@yarnpkg/cli/"),
    clean=lambda t: t.rsplit("/", 1)[-1],
)
