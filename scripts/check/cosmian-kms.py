#!/usr/bin/env python3
# cosmian-kms: GitHub releases, tag X.Y.Z (no v prefix). Window = latest patch of the
# last 3 minor lines, matching build.conf.
#
# Two traps this app carries, both worth re-checking on every bump (see build.conf):
#   - tags are ANNOTATED, so refs/tags/<v> is the tag object, not the commit
#   - rust-toolchain.toml on master does NOT match the tags; read it at the tag
from _lib import github, report

report("cosmian-kms", github("Cosmian/kms"))
