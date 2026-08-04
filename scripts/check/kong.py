#!/usr/bin/env python3
# kong (Kong Gateway OSS): GitHub tags, X.Y.Z, no v prefix.
#
# n=1 on purpose: Kong pins its whole native toolchain per release (OpenResty, OpenSSL,
# PCRE, six git-pinned modules) plus 39 patches written against that exact nginx
# version, so lines cannot share a build. See build.conf.
#
# NOTE: GitHub's tag list is sorted ALPHABETICALLY, so "3.10.0" would come before
# "3.9.0". _lib's github()/github_tags() sort numerically -- do not hand-eyeball the
# raw API output when checking what is newest.
from _lib import github_tags, report

report("kong", github_tags("Kong/kong"), n=1)
