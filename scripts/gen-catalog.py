#!/usr/bin/env -S uv run --quiet
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyyaml>=6"]
# ///
"""Generate catalog.lock.yaml: the actually-published state of the GHCR catalog.

For every `images/<app>` container package in the org, list its published tags
(version -> digest + publish date) and merge in the app-level metadata
(source/license/tier/status) from the hand-curated catalog.yaml.

  uv run scripts/gen-catalog.py            # write catalog.lock.yaml (fast: 1 gh call/pkg)
  uv run scripts/gen-catalog.py --meta     # also fetch per-image size + real arches + layers
  uv run scripts/gen-catalog.py --table    # also print a per-app/version table
  uv run scripts/gen-catalog.py --self-test

Auth/pagination/registry crawling are delegated to `gh` (installed + authed) --
ponytail: don't reimplement a registry client when `gh api --paginate` exists.
View a single app with: yq '.apps.jdk' catalog.lock.yaml
"""
import json
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from functools import lru_cache
from pathlib import Path

import yaml

ORG = "quenchworks"
ROOT = Path(__file__).resolve().parent.parent
CURATED = ROOT / "catalog.yaml"
OUT = ROOT / "catalog.lock.yaml"
# Every image in this factory is built amd64+arm64 (native, no QEMU). It's a
# build invariant, not per-tag data -- recording it here avoids N manifest
# inspects. ponytail: stamp the known invariant, add per-tag arch only if an
# app ever ships a single-arch image.
ARCHES = ["amd64", "arm64"]


def human(n: int) -> str:
    """bytes -> human size (e.g. 123.4 MB)."""
    f = float(n)
    for unit in ("B", "KB", "MB", "GB"):
        if f < 1024 or unit == "GB":
            return f"{f:.1f} {unit}"
        f /= 1024


def _raw(ref: str):
    r = subprocess.run(
        ["docker", "buildx", "imagetools", "inspect", ref, "--raw"],
        capture_output=True, text=True,
    )
    return json.loads(r.stdout) if r.returncode == 0 else None


@lru_cache(maxsize=None)
def image_meta(repo: str, digest: str) -> dict:
    """Per-image registry metadata: real arches + compressed amd64 size + layers.

    Two inspects per unique digest (index + the amd64 child manifest); the
    lru_cache dedupes the many tags that share a digest (e.g. `21` and
    `21.0.11`). ponytail: only runs under --meta -- the basic run stays one
    `gh` call per package.
    """
    idx = _raw(f"{repo}@{digest}")
    if not idx:
        return {}
    mans = [m for m in idx.get("manifests", [])
            if m.get("platform", {}).get("architecture", "unknown") != "unknown"]
    arches = sorted({m["platform"]["architecture"] for m in mans}) or ARCHES
    amd = next((m for m in mans if m["platform"]["architecture"] == "amd64"), None)
    meta = {"arches": arches}
    pm = _raw(f"{repo}@{amd['digest']}") if amd else idx  # single-arch: idx IS the manifest
    if pm and "layers" in pm:
        size = sum(l.get("size", 0) for l in pm["layers"]) + pm.get("config", {}).get("size", 0)
        meta["size"] = human(size)
        meta["layers"] = len(pm["layers"])
    return meta


def gh(path: str) -> list:
    """gh api --paginate <path> -> parsed JSON (list; pages concatenated)."""
    out = subprocess.run(
        ["gh", "api", "--paginate", path],
        capture_output=True, text=True, check=True,
    ).stdout
    # --paginate concatenates page arrays as `][` -> stitch back into one array.
    return json.loads(out.replace("][", ","))


def is_version_tag(tag: str) -> bool:
    """A real release tag, not a registry artifact (cosign sig, scan, arch leg)."""
    return (
        bool(tag) and tag[0].isdigit()       # real versions start with a digit
        and not tag.startswith("sha256-")
        and not tag.startswith("scan-")
        and not tag.endswith(("-amd64", "-arm64"))
    )


def is_full_version(tag: str) -> bool:
    """A full release tag (e.g. 8.0.127, 2026.04.0), not a bare major/minor alias
    like `8` or `1.26` that registries publish as moving pointers."""
    return is_version_tag(tag) and tag.count(".") >= 2


# Editorial fields carried straight through from catalog.yaml into the lock, so
# the lock is the single complete dataset the website reshapes (no second source).
EDITORIAL_FIELDS = ("category", "summary", "source", "upstream", "license", "tier", "status", "cleanAlternative", "knownIssue")


@lru_cache(maxsize=None)
def build_versions(slug: str) -> frozenset:
    """The exact VERSIONS declared in apps/<slug>/build.conf -- the source of truth
    for which tags belong to an app. Empty if the app has no build.conf (e.g. a
    chart-only/stack entry), in which case versions_of keeps its default behaviour."""
    conf = ROOT / "apps" / slug / "build.conf"
    if not conf.is_file():
        return frozenset()
    res = subprocess.run(
        ["bash", "-c", f'source "{conf}"; printf "%s\\n" "${{VERSIONS[@]}}"'],
        capture_output=True, text=True,
    )
    return frozenset(v for v in res.stdout.split() if v)


def versions_of(pkg: str, repo: str, with_meta: bool) -> list:
    """Published version tags for one package (newest first). Empty if unpublished.
    Only tags that EXACTLY match the app's build.conf VERSIONS are kept -- registry
    extras like the `8` / `latest` major-alias tags are dropped, so the catalog
    version list always mirrors build.conf."""
    try:
        raw = gh(f"/orgs/{ORG}/packages/container/{pkg.replace('/', '%2F')}/versions?per_page=100")
    except subprocess.CalledProcessError:
        return []  # planned app, no GHCR package yet

    # tag -> (digest, published) for every published version tag
    published = {}
    for v in raw:
        for tag in v.get("metadata", {}).get("container", {}).get("tags", []):
            if is_version_tag(tag):
                published.setdefault(tag, (v["name"], v["created_at"]))

    # Prefer the exact build.conf VERSIONS. If the image is stale (build.conf was
    # bumped but not rebuilt, so those exact tags aren't published), fall back to the
    # published FULL versions -- never the bare `8` / `1.26` major-alias tags, and
    # never empty just because build.conf ran ahead of the registry.
    allowed = build_versions(pkg.split("/")[-1])
    keep = [t for t in published if t in allowed]
    if not keep:
        keep = [t for t in published if is_full_version(t)]

    versions = []
    for tag in keep:
        digest, created = published[tag]
        row = {"version": tag, "digest": digest, "published": created}
        if with_meta:
            im = image_meta(repo, digest)
            row["size"] = im.get("size")
            row["layers"] = im.get("layers")
        versions.append(row)
    versions.sort(key=lambda x: x["published"], reverse=True)
    return versions


def _log(msg: str) -> None:
    print(f"\N{BULLET} {msg}", file=sys.stderr, flush=True)


def _bar(done: int, total: int, label: str) -> None:
    """One-line progress bar on stderr (stdout/the lock stay clean)."""
    if not total:
        return
    pct = done * 100 // total
    fill = "#" * (pct // 4)  # 25-char track
    print(f"\r  {label} [{fill:<25}] {pct:3d}% ({done}/{total})",
          end="\n" if done == total else "", file=sys.stderr, flush=True)


def collect(with_meta: bool = False) -> dict:
    # Iterate catalog.yaml (the editorial source of truth) so EVERY app lands in
    # the lock -- including planned ones with no published versions yet.
    rows = yaml.safe_load(CURATED.read_text())["catalog"]
    repo = lambda slug: f"ghcr.io/{ORG}/images/{slug}"

    # ponytail: the whole runtime is network wait. Parallelize it in two flat
    # fan-outs instead of per-app loops (which let a version-heavy app serialize
    # its own inspects). lru_cache is thread-safe; assembly below is cache-hot.
    # phase 1: list each app's published tags -- one gh call per app, concurrent.
    _log(f"phase 1/3: listing published tags for {len(rows)} apps")
    listed = [None] * len(rows)
    with ThreadPoolExecutor(max_workers=16) as ex:
        futs = {ex.submit(versions_of, f"images/{r['name']}", repo(r["name"]), False): i
                for i, r in enumerate(rows)}
        for n, f in enumerate(as_completed(futs), 1):
            i = futs[f]
            listed[i] = (rows[i], f.result())  # index back -> keep catalog.yaml order
            _bar(n, len(rows), "tags")

    if with_meta:
        # phase 2: warm per-image metadata for every UNIQUE digest at once, so the
        # ~300 inspects finish in ~ceil(N/32) waves, not app-by-app. 32 workers;
        # raise it until GHCR rate-limits.
        pairs = sorted({(repo(r["name"]), v["digest"]) for r, vs in listed for v in vs})
        _log(f"phase 2/3: inspecting {len(pairs)} unique image digests")
        with ThreadPoolExecutor(max_workers=32) as ex:
            futs = [ex.submit(image_meta, rp, dg) for rp, dg in pairs]
            for n, _f in enumerate(as_completed(futs), 1):
                _bar(n, len(pairs), "meta")

    _log("phase 3/3: assembling lock (cache-hot, no network)")
    apps = {}
    for r, versions in listed:
        rp = repo(r["name"])
        if with_meta:
            for v in versions:  # cache-hot from phase 2 -> no network here
                im = image_meta(rp, v["digest"])
                v["size"] = im.get("size")
                v["layers"] = im.get("layers")
            arches = (image_meta(rp, versions[0]["digest"]).get("arches", ARCHES)
                      if versions else ARCHES)
        else:
            arches = ARCHES
        app = {"image": rp}
        for k in EDITORIAL_FIELDS:
            if r.get(k) is not None:
                app[k] = r[k]
        app["arches"] = arches
        app["versions"] = versions
        apps[r["name"]] = app
    return {"registry": f"ghcr.io/{ORG}/images", "apps": apps}


def table(cat: dict) -> str:
    rows = ["APP                VERSION       PUBLISHED             SIZE       DIGEST"]
    for app, d in cat["apps"].items():
        for v in d["versions"]:
            size = v.get("size") or "-"
            rows.append(f"{app:<18} {v['version']:<13} {v['published']:<21} {size:<10} {v['digest'][:19]}")
    return "\n".join(rows)


def self_test():
    keep = ["25.0.3", "1.22.22", "4.16.0-dev", "20260621", "3.9.5"]
    drop = ["latest", "sha256-abc", "scan-amd64", "25.0.3-amd64", "25.0.3-arm64"]
    assert all(is_version_tag(t) for t in keep), "dropped a real version"
    assert not any(is_version_tag(t) for t in drop), "kept a registry artifact"
    # gh --paginate stitch: two pages of one-element arrays -> two elements.
    assert json.loads('[{"a":1}][{"a":2}]'.replace("][", ",")) == [{"a": 1}, {"a": 2}]
    print("self-test OK")


def main():
    if "--self-test" in sys.argv:
        return self_test()
    cat = collect(with_meta="--meta" in sys.argv)
    # ponytail: kill YAML anchors (&id001/*id001) -- the repeated arches list
    # gets deduplicated into an alias otherwise, which reads as noise.
    class NoAlias(yaml.SafeDumper):
        def ignore_aliases(self, data):
            return True

    OUT.write_text(
        "# GENERATED by scripts/gen-catalog.py -- do not edit by hand.\n"
        "# Published state of the GHCR catalog (run the script to refresh).\n"
        + yaml.dump(cat, Dumper=NoAlias, sort_keys=False, default_flow_style=False, width=100)
    )
    n = sum(len(a["versions"]) for a in cat["apps"].values())
    print(f"wrote {OUT.relative_to(ROOT)}: {len(cat['apps'])} apps, {n} published versions")
    if "--table" in sys.argv:
        print(table(cat))


if __name__ == "__main__":
    main()
