#!/usr/bin/env python3
# flink: Apache release directories (flink-X.Y.Z/). Newest-only, like the recipe.
from _lib import scrape, report
report("flink", scrape("https://dlcdn.apache.org/flink/", r'href="flink-(\d+\.\d+\.\d+)/"'), n=1)
