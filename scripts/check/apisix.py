#!/usr/bin/env python3
# apisix (Apache APISIX): GitHub releases, X.Y.Z, no v prefix.
#
# n=1 on purpose. APISIX pins its whole native runtime per release in `.requirements`
# (APISIX_RUNTIME -> a specific OpenResty + apisix-nginx-module + module roster), and
# apisix-nginx-module's patch.sh matches the OpenResty version EXACTLY, so lines cannot
# share a build. Same call as scripts/check/kong.py. See apps/apisix/build.conf.
#
# THE TAG IS ONLY ONE THIRD OF THE ANSWER. A release can move APISIX_RUNTIME or
# APISIX_DASHBOARD_COMMIT without changing anything this checker's version compare can
# see, and a stale runtime pin is exactly the kind of drift that ships a subtly wrong
# image. So after the normal report() this also re-reads `.requirements` AT THE NEWEST
# UPSTREAM TAG and diffs it against build.conf. Values are always read at the tag,
# never from master -- master's runtime pin does not belong to any release.
import re
import subprocess
import sys

from _lib import BASE, github, report, vkey

APP = "apisix"


def conf_var(name):
    """One associative-array entry from apps/apisix/build.conf, for our newest version."""
    out = subprocess.run(
        ["bash", "-c",
         f"source {BASE}/apps/{APP}/build.conf; "
         f'v="${{VERSIONS[${{#VERSIONS[@]}}-1]}}"; echo "${{{name}[$v]}}"'],
        capture_output=True, text=True).stdout.strip()
    return out


def requirements_at(tag):
    """APISIX_RUNTIME / APISIX_DASHBOARD_COMMIT from .requirements at a given tag."""
    out = subprocess.run(
        ["gh", "api", f"repos/apache/apisix/contents/.requirements?ref={tag}",
         "-H", "Accept: application/vnd.github.raw"],
        capture_output=True, text=True).stdout
    vals = dict(re.findall(r"^(APISIX_RUNTIME|APISIX_DASHBOARD_COMMIT)=(\S+)$",
                           out, re.M))
    return vals


behind = report(APP, github("apache/apisix"), n=1)

# The pin audit runs against the newest tag upstream has, so it reports drift whether or
# not we are behind on the version itself.
tags = sorted({t for t in github("apache/apisix") if re.match(r"\d", t)},
              key=vkey, reverse=True)
if not tags:
    print("  !! could not list upstream tags -- pin audit skipped")
    sys.exit(0)

newest = tags[0]
want = requirements_at(newest)
if not want:
    print(f"  !! could not read .requirements at {newest} -- pin audit skipped")
    sys.exit(0)

print(f"  .requirements @ {newest}:")
drift = []
for var, key in (("APISIX_RUNTIME", "RUNTIME"),
                 ("APISIX_DASHBOARD_COMMIT", "DASHBOARD_COMMIT")):
    have = conf_var(key)
    upstream = want.get(var, "?")
    ok = have == upstream
    if not ok:
        drift.append(f"{key}: {have or '<unset>'} -> {upstream}")
    print(f"    {var:<24s} have={have or '<unset>'}  upstream={upstream}  "
          f"{'ok' if ok else 'DRIFT'}")

# The runtime pin also decides which OpenResty + apisix-nginx-module we build, and
# those live in api7's build script, not in APISIX's tree -- so a runtime bump means
# re-reading build-apisix-runtime.sh, not just editing a number.
if drift:
    print("  => PIN DRIFT " + str(drift))
    print("     A RUNTIME change means re-reading "
          "https://raw.githubusercontent.com/api7/apisix-build-tools/"
          f"apisix-runtime/{want.get('APISIX_RUNTIME', '?')}/build-apisix-runtime.sh "
          "for the new OpenResty / module / patch versions.")
elif not behind:
    print("  => pins ok")
