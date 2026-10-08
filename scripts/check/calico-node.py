#!/usr/bin/env python3
# calico-node: Go binaries built from projectcalico/calico tags, laid over Wolfi's
# calico-node-<minor> package, so a version is buildable only once Wolfi ships its line.
# Report the newest calico-node-<minor> Wolfi has (upstream can be a minor ahead).
from _lib import wolfi_match, report

report("calico-node", list(wolfi_match(r"^calico-node-\d+\.\d+$").values()), n=1)
