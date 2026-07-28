#!/usr/bin/env python3
# vault (HashiCorp Vault): GitHub releases (hashicorp/vault). We ship the newest patch of
# each SHIPPABLE stable MINOR line, so compare per-line at depth=2 (the X.Y line),
# matching the recipe's VERSIONS. Enterprise builds are cut as `+ent` tags in the same
# repo -- keep only bare X.Y.Z.
#
# We ship TWO lines, not three: 1.20.x links a docker/docker HIGH (CVE-2026-34040) that
# cannot be floated on the `+incompatible` module path -- see apps/vault/build.conf.
# report_lines keys off the current VERSIONS, so it tracks whatever we ship and still
# flags a brand-new upstream line.
#
# NOTE: only Vault <= 1.14.x was MPL-2.0; every line we ship is BUSL-1.1, which is NOT
# OSI-approved open source. OpenBao is the truly-open fork (scripts/check/openbao.py).
from _lib import github, report_lines

report_lines(
    "vault",
    github("hashicorp/vault"),
    depth=2,
    keep=lambda t: t.replace(".", "").isdigit(),
)
