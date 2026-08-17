#!/usr/bin/env python3
"""Bump an app's build.conf to a new version, computing whatever pins it needs.

    uv run scripts/bump.py <app> <version> [<version> ...]

Rewrites VERSIONS to the versions given (newest last) and fills in every
`declare -A` map the recipe uses, deriving each value from the real upstream
artifact rather than copying a neighbouring entry:

    SHA256 / SHA512      sha of the tarball at melange.yaml's `uri:`
    SHA_AMD / SHA_ARM    per-arch shas (uri is resolved per architecture)
    COMMIT               the git commit a tag points at
    PIN                  the exact Wolfi apk revision (name=ver-rN)

It does NOT touch anything it cannot derive. Maps it does not understand are
left alone and reported, so an app with a bespoke pin (kong's OPENRESTY,
n8n-runners' JSSHA, dotnet's TFM, ...) fails loudly here instead of silently
shipping a stale hash.

This exists because a 60-app update round is otherwise 60 hand-edits, and a
hand-copied hash that happens to be valid for the WRONG version is exactly the
kind of error the 0-CVE gate cannot catch -- it builds fine, it just builds the
previous release.
"""
from __future__ import annotations

import hashlib
import pathlib
import re
import subprocess
import sys
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent

# Every value written to a map must match its expected shape. Without this a failed
# lookup silently writes its ERROR BODY into the recipe: gh prints a 404 JSON blob on
# stdout, which is non-empty and therefore looks like success. That is how a pin ends
# up as {"message":"Not Found"} and the build fails somewhere far away from the cause.
HEX40 = re.compile(r"[0-9a-f]{40}")
SHAPES = {
    "SHA256": re.compile(r"[0-9a-f]{64}"),
    "SHA512": re.compile(r"[0-9a-f]{128}"),
    # 64 OR 128: some recipes (elasticsearch) verify sha512 in these maps.
    "SHA_AMD": re.compile(r"[0-9a-f]{64}([0-9a-f]{64})?"),
    "SHA_ARM": re.compile(r"[0-9a-f]{64}([0-9a-f]{64})?"),
    "COMMIT": HEX40,
    "PIN": re.compile(r"[\w.+~-]+-r\d+"),
    "PKG": re.compile(r"[a-z][\w.+-]*"),
    "SRC_SHA": re.compile(r"[0-9a-f]{64}"),
}


def uri_for_placeholder(app: str, version: str, ph: str) -> str | None:
    """The uri whose `expected-sha256:` is this map's placeholder.

    Recipes often fetch SEVERAL things (a source tarball AND per-arch prebuilt
    binaries), so "the first uri in the file" is the wrong artifact for every map but
    one -- and hashing the wrong artifact yields a perfectly valid sha that fails only
    at build time. Pair each placeholder with its own uri instead.
    """
    mel = ROOT / "apps" / app / "melange.yaml"
    if not mel.exists():
        return None
    text = mel.read_text()
    for m in re.finditer(r'uri:\s*"?(\S+?)"?\s*\n\s*expected-sha(?:256|512):\s*__(\w+)__', text):
        if m.group(2) == ph:
            u = m.group(1).replace("${{package.version}}", version).replace("__VER__", version)
            return None if "${" in u or "__" in u else u
    return None


def pin_pkg_names(conf_text: str, version: str, mel_text: str = "") -> list[str]:
    """Package names a PIN recipe actually installs, read from its render().

    The PIN VALUE is only the apk revision (2.6.14-r0); the NAME is assembled in
    render() and is usually version-qualified -- openldap-2.6, not openldap, and
    ingress-nginx-controller-${v%.*}. Looking the bare app name up in the index
    therefore finds nothing, which reads as "no such release" rather than "wrong
    package name".
    """
    mm = ".".join(version.split(".")[:2])
    names = []
    # melange installs the package by NAME (`- gradle-9=__GRADLEVER__`); the build.conf
    # render() may only carry the revision, so search both files.
    for raw in re.findall(r"^\s*-\s+([A-Za-z][\w.+-]*)=__\w+__", mel_text, re.M):
        names.append(raw)
    for raw in re.findall(r"\|([A-Za-z][\w.+${}%*\\-]*)=\$", conf_text):
        n = raw.replace("${v%.*}", mm).replace("${v}", version)
        if "$" in n or "{" in n:
            continue
        names.append(n)
    return names


def sh(*args: str) -> str:
    return subprocess.run(args, capture_output=True, text=True).stdout.strip()


def fetch_sha(url: str, algo: str = "sha256") -> str:
    h = hashlib.new(algo)
    # Some CDNs (fastdl.mongodb.org among them) 403 the default urllib agent.
    req = urllib.request.Request(url, headers={"User-Agent": "curl/8.0"})
    n = 0
    with urllib.request.urlopen(req, timeout=900) as r:
        for chunk in iter(lambda: r.read(1 << 20), b""):
            h.update(chunk); n += len(chunk)
    # An empty body still hashes to a valid-looking 64-hex value
    # (e3b0c442...b855 for sha256), which passes every shape check and pins NOTHING.
    # A missing per-arch asset must fail here, not silently become a pin.
    if n == 0:
        raise RuntimeError(f"empty response from {url} -- asset missing?")
    return h.hexdigest()


def arch_vars(text: str) -> dict[str, dict[str, str]]:
    """Read the recipe's own `case $(uname -m)` arch mapping.

    Recipes name their arch token differently per app (garch=amd64, esarch=x86_64,
    marsh=x86_64, ...), so the only reliable source is the case statement itself:
        x86_64)  garch=amd64 ; sha=__SHA_AMD__ ;;
        aarch64) garch=arm64 ; sha=__SHA_ARM__ ;;
    Returns {"amd": {var: value}, "arm": {var: value}}.
    """
    out = {"amd": {}, "arm": {}}
    for host, key in (("x86_64", "amd"), ("aarch64", "arm")):
        for m in re.finditer(rf"{host}\)(.*?);;", text, re.S):
            for var, val in re.findall(r"(\w+)=([\w.-]+)", m.group(1)):
                out[key].setdefault(var, val)
    return out


def melange_uris(app: str, version: str, arch: str = "amd") -> list[str]:
    """Every fetch/download URL in the recipe, fully resolved for one arch.

    Beyond ${{package.version}}, recipes interpolate plain shell vars (${VER} and an
    arch token). Leaving those literal produced URLs that 404 -- which looked like the
    release was missing rather than like a resolver bug, so resolve them properly.
    """
    mel = ROOT / "apps" / app / "melange.yaml"
    if not mel.exists():
        return []
    text = mel.read_text()
    amap = arch_vars(text).get(arch, {})
    urls = re.findall(r'(?:uri|url|binurl|srvurl)[:=]\s*"?(https://[^\s"\']+)', text)
    out = []
    for u in urls:
        u = u.replace("${{package.version}}", version).replace("__VER__", version)
        u = re.sub(r"\$\{VER\}|\$VER\b", version, u)
        for var, val in amap.items():
            u = u.replace("${" + var + "}", val).replace("$" + var, val)
        if "${" in u or "__" in u:       # still unresolved -> not derivable, say so
            continue
        out.append(u)
    return out


def per_arch(url: str, arch: str) -> str:
    """Resolve a URL for one architecture the way the recipe's case-stmt does."""
    amd, arm = ("x86_64", "amd64", "x86-64", "x64"), ("aarch64", "arm64", "arm_64")
    for a in (amd if arch == "amd" else arm):
        for b in (arm if arch == "amd" else amd):
            if b in url:
                return url.replace(b, a)
    return url


def tag_commit(app: str, version: str) -> str | None:
    mel = (ROOT / "apps" / app / "melange.yaml").read_text()
    m = re.search(r"repository:\s*https://github\.com/([^\s/]+/[^\s/.]+)", mel)
    if not m:
        return None
    repo = m.group(1)
    # (?<![\w-]) so `dashboard-tag:`, `ui-tag:`, `web-ui-tag:` etc do NOT match. A bare
    # `tag:` search grabbed meilisearch's `dashboard-tag: v0.4.1` and pinned the
    # DASHBOARD's commit as the app's -- a real sha for the wrong thing, which melange
    # only catches because it verifies expected-commit against what it checked out.
    tagexpr = re.search(r"(?<![\w-])tag:\s*(\S+)", mel)
    tag = (tagexpr.group(1) if tagexpr else "${{package.version}}")
    tag = tag.replace("${{package.version}}", version).replace("__VER__", version).strip('"')
    for cand in (tag, f"v{version}", version):
        sha = sh("gh", "api", f"repos/{repo}/git/ref/tags/{cand}", "--jq", ".object.sha")
        typ = sh("gh", "api", f"repos/{repo}/git/ref/tags/{cand}", "--jq", ".object.type")
        if not HEX40.fullmatch(sha):
            continue                      # 404 bodies are non-empty; only a real sha counts
        if typ != "tag":                  # lightweight tag -> already the commit
            return sha
        # annotated tag -> deref the tag object to its commit
        deref = sh("gh", "api", f"repos/{repo}/git/tags/{sha}", "--jq", ".object.sha")
        if HEX40.fullmatch(deref):
            return deref
    return None


def wolfi_pin(pkg: str, version: str) -> str | None:
    idx = pathlib.Path("/tmp/APKINDEX.txt")
    if not idx.exists():
        subprocess.run(
            "curl -s https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz "
            "| tar -xzO APKINDEX > /tmp/APKINDEX.txt", shell=True, check=False)
    text = idx.read_text(errors="replace")
    best = None
    for blk in text.split("\n\n"):
        if f"\nP:{pkg}\n" in "\n" + blk:
            v = re.search(r"^V:(.+)$", blk, re.M)
            if v and v.group(1).split("-r")[0] == version:
                rev = int(v.group(1).split("-r")[1])
                if best is None or rev > best[1]:
                    best = (v.group(1), rev)
    return best[0] if best else None


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__)
        return 2
    app, versions = sys.argv[1], sys.argv[2:]
    conf = ROOT / "apps" / app / "build.conf"
    text = conf.read_text()

    maps = re.findall(r"declare -A (\w+)=\(", text)
    known = {"SHA256", "SHA512", "SHA_AMD", "SHA_ARM", "COMMIT", "PIN", "PKG", "SRC_SHA"}
    unknown = [m for m in maps if m not in known]
    if unknown:
        print(f"!! {app}: unhandled map(s) {unknown} -- bump by hand")
        return 3

    newest = versions[-1]
    entries: dict[str, dict[str, str]] = {m: {} for m in maps}

    for v in versions:
        uris = melange_uris(app, v)
        for m in maps:
            if m == "SRC_SHA":
                u = uri_for_placeholder(app, v, "SRC_SHA")
                if not u:
                    print(f"!! {app}: no uri paired with __SRC_SHA__ for {v}")
                    return 3
                entries[m][v] = fetch_sha(u)
            elif m in ("SHA256", "SHA512"):
                u = uri_for_placeholder(app, v, m)
                if u:
                    entries[m][v] = fetch_sha(u, "sha256" if m == "SHA256" else "sha512")
                    continue
                if not uris:
                    print(f"!! {app}: no resolvable uri for {v}")
                    return 3
                algo = "sha256" if m == "SHA256" else "sha512"
                entries[m][v] = fetch_sha(uris[0], algo)
            elif m in ("SHA_AMD", "SHA_ARM"):
                if not uris:
                    print(f"!! {app}: no resolvable uri for {v}")
                    return 3
                a = "amd" if m == "SHA_AMD" else "arm"
                au = melange_uris(app, v, a) or [per_arch(uris[0], a)]
                # Recipes that fetch BOTH a source archive and per-arch binaries list the
                # source first, so au[0] is the wrong artifact -- it yields a valid sha of
                # the SOURCE tarball, which only fails much later as a checksum mismatch.
                # Prefer a url that actually carries this arch's token.
                tok = ("amd64", "x86_64", "x86-64", "x64") if a == "amd" else ("arm64", "aarch64", "arm_64")
                picked = next((u for u in au if any(t in u for t in tok)), au[0])
                # The DIGEST ALGORITHM is per-recipe, not per-map-name: elasticsearch keeps
                # sha512 values in SHA_AMD/SHA_ARM. Hardcoding sha256 writes a 64-char hash
                # the recipe then fails to verify, surfacing only as a checksum FAILED.
                #
                # Read it from the RECIPE (which sha*sum it pipes into), not from the value
                # already in the map -- once a wrong-width value has been written, inferring
                # from it just reproduces the same mistake.
                melp = ROOT / "apps" / app / "melange.yaml"
                mtext = melp.read_text() if melp.exists() else ""
                algo = "sha512" if "sha512sum -c" in mtext else "sha256"
                entries[m][v] = fetch_sha(picked, algo)
            elif m == "COMMIT":
                c = tag_commit(app, v)
                if not c:
                    print(f"!! {app}: could not resolve tag->commit for {v}")
                    return 3
                entries[m][v] = c
            elif m == "PIN":
                melp = ROOT / "apps" / app / "melange.yaml"
                cands = pin_pkg_names(text, v, melp.read_text() if melp.exists() else "") or [app]
                p = next((x for x in (wolfi_pin(n, v) for n in cands) if x), None)
                if not p:
                    print(f"!! {app}: none of {cands} at {v} found in the Wolfi index")
                    return 3
                entries[m][v] = p
            elif m == "PKG":
                # PKG maps a version to its Wolfi package NAME (1.26.5 -> go-1.26).
                # Carry the existing naming pattern forward with this version's line.
                prev = re.search(r"declare -A PKG=\(\s*\n\s*\[[^\]]+\]=(\S+)", text)
                if not prev:
                    print(f"!! {app}: cannot infer the PKG naming pattern")
                    return 3
                mm = ".".join(v.split(".")[:2])
                entries[m][v] = re.sub(r"\d+\.\d+$", mm, prev.group(1))

    text = re.sub(r"^VERSIONS=\(.*\)$", "VERSIONS=(" + " ".join(versions) + ")",
                  text, count=1, flags=re.M)
    for m in maps:
        body = "\n".join(f"  [{v}]={entries[m][v]}" for v in versions)
        block = f"declare -A {m}=(\n{body}\n)"
        # Maps come in BOTH layouts and only rewriting one of them is silently fatal:
        # VERSIONS moves, the map keeps its old keys, and the build dies later on
        # `PIN[$1]: unbound variable` -- before the scanner runs, so it looks nothing
        # like a version problem.
        multi = rf"declare -A {m}=\(\n(?:\s*\[[^\]]+\]=\S*\n)*\)"
        single = rf"declare -A {m}=\((?:\s*\[[^\]]+\]=\S*)+\s*\)"
        if re.search(multi, text):
            text = re.sub(multi, block, text, count=1)
        elif re.search(single, text):
            text = re.sub(single, block, text, count=1)
        else:
            print(f"!! {app}: could not locate the {m} map to rewrite")
            return 3

    for m in maps:
        for v, val in entries[m].items():
            if not SHAPES[m].fullmatch(val):
                print(f"!! {app}: refusing to write {m}[{v}]={val[:60]!r} -- wrong shape")
                return 3

    conf.write_text(text)
    print(f"ok {app} -> {' '.join(versions)}" + (f"  [{', '.join(maps)}]" if maps else ""))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
