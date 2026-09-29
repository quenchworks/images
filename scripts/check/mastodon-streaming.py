#!/usr/bin/env python3
# mastodon-streaming: same release tags as mastodon (GitHub releases of mastodon/mastodon). Newest only.
from _lib import github, report
report("mastodon-streaming", github("mastodon/mastodon"), n=1)
