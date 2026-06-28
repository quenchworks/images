#!/usr/bin/env python3
# redis: build fetches download.redis.io, but versions = GitHub releases (redis/redis).
from _lib import github, report
report("redis", github("redis/redis"))
