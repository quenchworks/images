#!/usr/bin/env python3
# static: minimal static base image, versioned by build DATESTAMP (e.g. 20260621),
# not an upstream release. There is no upstream version to diff against, so it is
# always "current" by definition. Emit a normal `have=` line (classified ok, NOT an
# error) so the runner doesn't flag it; rebuild on a cadence to refresh the base.
import re
import pathlib

conf = pathlib.Path(__file__).resolve().parents[2] / "apps" / "static" / "build.conf"
m = re.search(r"VERSIONS=\(([^)]*)\)", conf.read_text()) if conf.exists() else None
ver = m.group(1).split()[-1].strip() if m and m.group(1).split() else "datestamped"
print(f"static               have={ver:>13}  latest={ver:>13}  ok  (datestamped base, no upstream to track)")
