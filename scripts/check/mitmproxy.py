#!/usr/bin/env python3
# mitmproxy: PyPI releases (the recipe pip-installs mitmproxy==<ver>), newest-only like the recipe.
# A new release can move the mitmproxy_rs pin: build.conf's RS map must follow.
from _lib import pypi, report
report("mitmproxy", pypi("mitmproxy"), n=1)
