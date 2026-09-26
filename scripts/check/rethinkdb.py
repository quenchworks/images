#!/usr/bin/env python3
# rethinkdb: the Wolfi rethinkdb apk (FROM_SOURCE=0), newest-only like the recipe.
from _lib import wolfi, report
report("rethinkdb", wolfi("rethinkdb"), n=1)
