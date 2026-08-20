#!/usr/bin/env python3
# apisix-ingress-controller: the ASF source-release listing, NOT GitHub releases.
#
# The recipe fetches archive.apache.org/dist/apisix/ingress-controller/<v>/
# apache-apisix-ingress-controller-<v>-src.tgz, and upstream cuts GitHub tags that
# never get an ASF source release (2.0.0, 2.0.1 and 2.1.0 have tags but no dist
# directory). Checking GitHub releases would therefore propose a version whose
# tarball 404s -- the rustfs failure shape. Scraping the dist listing encodes
# exactly what build.conf can fetch.
#
# n=1: latest only. Each minor pins its own Gateway API bundle (2.1 -> v1.3.0,
# 2.2 -> v1.6.0) and its own adc sidecar, and the catalog ships ONE pinned
# gateway-api-crds chart. See build.conf.
from _lib import scrape, report

report(
    "apisix-ingress-controller",
    scrape("https://archive.apache.org/dist/apisix/ingress-controller/",
           r'href="(\d+\.\d+\.\d+)/"'),
    n=1,
)
