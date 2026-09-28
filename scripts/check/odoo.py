#!/usr/bin/env python3
# odoo: dated nightly builds of the newest line on nightly.odoo.com (19.0.YYYYMMDD).
# Odoo tags no patch releases; see apps/odoo/build.conf.
from _lib import scrape, report
report("odoo", ["19.0." + d for d in scrape("https://nightly.odoo.com/19.0/nightly/src/", r'odoo_19\.0\.([0-9]{8})\.tar\.gz')], n=1)
