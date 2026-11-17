# Envoy dependency inventory as CycloneDX: every Bazel module linked into envoy-static that
# deps.yaml gives a CPE, at its resolved version. Usage: inventory.py <share-dir>
import json, re, sys, yaml
d = sys.argv[1]
deps = yaml.safe_load(open(f"{d}/deps.yaml"))
linked = {l.strip().lstrip("@").split("+")[0] for l in open(f"{d}/linked-repos.txt") if l.strip()}
graph = json.load(open(f"{d}/bazel-mod-graph.json"))
ver = {}
def walk(n):
    for c in n.get("dependencies", []) + n.get("indirectDependencies", []):
        k = c.get("key", "")
        if "@" in k:
            name, v = k.split("@", 1)
            ver.setdefault(name, v)
        walk(c)
walk(graph)
comps, skipped = [], []
for key, m in deps.items():
    cpe = m.get("cpe")
    if not cpe or cpe == "N/A":
        continue
    names = {key, key.replace("_", "-"), m.get("apparent_name", key)}
    if not names & linked:
        continue
    v = next((ver[n] for n in names if n in ver), None)
    num = re.match(r"v?(\d+(?:\.\d+)+)", v or "")
    if not num or num.group(1).startswith("0.0.0"):
        skipped.append(f"{key}@{v}")
        continue
    vendor, product = cpe.split(":")[3:5]
    comps.append({"type": "library", "name": key, "version": num.group(1),
                  "purl": f"pkg:generic/{key}@{num.group(1)}",
                  "cpe": f"cpe:2.3:a:{vendor}:{product}:{num.group(1)}:*:*:*:*:*:*:*",
                  "properties": [{"name": "bazel:module-version", "value": v}]})
bom = {"bomFormat": "CycloneDX", "specVersion": "1.5", "version": 1, "components": comps,
       "properties": [{"name": "envoy:unversioned", "value": " ".join(sorted(skipped))}]}
json.dump(bom, open(f"{d}/inventory.cdx.json", "w"), indent=1)
print(f"inventory: {len(comps)} components with CPEs; no release version: {' '.join(sorted(skipped)) or 'none'}")
# 16 on 1.40.2: far fewer means the linked-repo list broke (build 6 shipped 3), not a smaller Envoy
assert len(comps) >= 10, f"inventory has only {len(comps)} components"
