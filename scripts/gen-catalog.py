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
    return not (
        tag == "latest"
        or tag.startswith("sha256-")
        or tag.startswith("scan-")
        or tag.endswith(("-amd64", "-arm64"))
    )


def collect(with_meta: bool = False) -> dict:
    meta = {
        r["name"]: r
        for r in yaml.safe_load(CURATED.read_text())["catalog"]
    }
    pkgs = [
        p["name"] for p in gh(f"/orgs/{ORG}/packages?package_type=container&per_page=100")
        if p["name"].startswith("images/")
    ]
    apps = {}
    for pkg in sorted(pkgs):
        app = pkg.split("/", 1)[1]
        repo = f"ghcr.io/{ORG}/{pkg}"
        versions = []
        for v in gh(f"/orgs/{ORG}/packages/container/{pkg.replace('/', '%2F')}/versions?per_page=100"):
            for tag in v.get("metadata", {}).get("container", {}).get("tags", []):
                if is_version_tag(tag):
                    row = {"version": tag, "digest": v["name"], "published": v["created_at"]}
                    if with_meta:
                        im = image_meta(repo, v["name"])
                        row["size"] = im.get("size")
                        row["layers"] = im.get("layers")
                    versions.append(row)
        if not versions:
            continue
        versions.sort(key=lambda x: x["published"], reverse=True)
        # Real arches from the newest image's manifest when --meta; else the invariant.
        arches = image_meta(repo, versions[0]["digest"]).get("arches", ARCHES) if with_meta else ARCHES
        m = meta.get(app, {})
        apps[app] = {
            "image": repo,
            "source": m.get("source"),
            "license": m.get("license"),
            "tier": m.get("tier"),
            "arches": arches,
            "versions": versions,
        }
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
