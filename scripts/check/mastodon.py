#!/usr/bin/env python3
# mastodon: GitHub releases of mastodon/mastodon, the tag archive the recipe fetches. Newest only.
from _lib import github, report
report("mastodon", github("mastodon/mastodon"), n=1)
