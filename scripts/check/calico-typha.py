#!/usr/bin/env python3
# calico-typha: built from the projectcalico/calico tag that matches calico-node, which sits on Wolfi's
# calico-node-<minor> package, so a version is buildable only once Wolfi ships its line.
# Report the newest calico-node-<minor> Wolfi has (upstream can be a minor ahead).
from _lib import wolfi_match, report

report("calico-typha", list(wolfi_match(r"^calico-node-\d+\.\d+$").values()), n=1)
