#!/usr/bin/env python3
# solr: apache dist; versions = GitHub tags releases/solr/X.Y.Z (apache/solr).
import re
from _lib import github_tags, report
report("solr", github_tags("apache/solr"),
       keep=lambda t: t.startswith("releases/solr/"), clean=lambda t: t.replace("releases/solr/", ""))
