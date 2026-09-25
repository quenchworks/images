#!/usr/bin/env python3
# wiremock: Maven Central releases of org.wiremock:wiremock-jetty12 (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import scrape, report
report("wiremock", scrape("https://repo1.maven.org/maven2/org/wiremock/wiremock-jetty12/maven-metadata.xml",
                          r"<version>(\d+\.\d+\.\d+)</version>"), n=1)
