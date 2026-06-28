#!/usr/bin/env python3
# postgres-documentdb: built from the FerretDB/documentdb FORK (not microsoft/),
# tags v<ver>-ferretdb-<x.y.z>. The image version is the documentdb extension
# version; strip the -ferretdb-* suffix to compare.
from _lib import github, report
report("postgres-documentdb", github("FerretDB/documentdb"),
       clean=lambda t: t.split("-ferretdb")[0])
