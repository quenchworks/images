# Floating Go modules past a CVE

Go recipes clear advisories in transitive modules by raising them before the
build:

```sh
go get golang.org/x/net@v0.56.0 golang.org/x/text@v0.39.0 …
```

**A `go get` line in a recipe is not evidence that the shipped binary got the
fix.** It can fail in two opposite directions, both silent, both leaving a green
build that ships the vulnerable module.

## 1. It can be a NO-OP — a `replace` directive outranks `go get`

If upstream's `go.mod` pins the module down with a `replace`, MVS honours the
replace and discards the float. `go get` still prints `upgraded`, so the build
log looks correct.

Authelia 4.39.20 shipped this way for weeks. Its `go.mod` had:

```
replace (
	filippo.io/edwards25519 v1.1.0 => filippo.io/edwards25519 v1.2.0
	golang.org/x/net => golang.org/x/net v0.55.0
)
```

and the published binary recorded, verbatim:

```
$ go version -m /usr/bin/authelia
	dep	golang.org/x/net	v0.56.0
	=>	golang.org/x/net	v0.55.0
```

The require said v0.56.0. The linked code was v0.55.0, i.e. still
CVE-2026-46600. Fix: drop the replace before floating, and only the ones that
hold a module *down* —

```sh
go mod edit -dropreplace=golang.org/x/net
```

Check every replace in the real tarball at the pinned version before assuming
there are none:

```sh
curl -sL https://github.com/<org>/<repo>/archive/refs/tags/v<VER>.tar.gz \
  | tar xz --wildcards -O '*/go.mod' | grep -n -A20 '^replace'
```

## 2. It can be a DOWNGRADE — the pin inverts once upstream moves past it

`go get mod@vX` rewrites the require to **exactly** vX. The moment upstream's own
`go.mod` requires more than vX, that line stops being a fix and becomes a
regression: it drags the module back down and re-introduces the CVE the float
was added to clear.

SpiceDB 1.56.0 requires x/net 0.57.0, x/text 0.40.0, x/crypto 0.54.0, x/sys
0.47.0. The house floor line (`go get golang.org/x/net@v0.56.0 …`) would have
lowered four modules.

So a hardcoded `go get mod@vX` is **not safe to leave in a recipe forever**.
Treat the version as a *floor* and only float what is actually below it.

## 3. When you bump an app's version, re-check its float floors

A version bump plus a stale float pin is a silent CVE regression — see (2). Read
the new tarball's `go.mod`, compare each floor against what upstream now
requires, and drop or raise floors that upstream has overtaken.

## The snippet

Copy this verbatim into the `runs:` block that builds the app. Start the block
with `set -eu` — without it a failed `go get` or `go mod vendor` does not stop
the script, and `go build` happily ships the un-floated tree.

```sh
set -eu
# ---- 0-CVE module float (docs/go-module-float.md) ----------------------
# FLOOR is a FLOOR, not a pin. A bare `go get mod@vX` fails silently in two
# directions: it is a NO-OP when an upstream `replace` overrides it, and it
# becomes a DOWNGRADE once upstream requires more than vX -- re-introducing
# the CVE it was added to clear. So float only what is below the floor, then
# assert the SHIPPED BINARY carries nothing below it.
FLOOR="golang.org/x/net@v0.56.0 golang.org/x/text@v0.39.0 golang.org/x/mod@v0.40.0 google.golang.org/grpc@v1.82.1"
MODS="$(echo "$FLOOR" | tr ' ' '\n' | cut -d@ -f1)"
# prints "<mod>@<floor> (<tag> has <ver>)" for every module below its floor
below_floor() {
  awk -v floor="$FLOOR" -v tag="$1" '
    BEGIN { n = split(floor, f, " ")
            for (i = 1; i <= n; i++) { split(f[i], kv, "@"); want[kv[1]] = kv[2] } }
    ($1 in want) {
      split(substr($2, 2), g, "."); split(substr(want[$1], 2), w, ".")
      for (i = 1; i <= 3; i++) {
        if ((g[i] + 0) > (w[i] + 0)) next
        if ((g[i] + 0) < (w[i] + 0)) { print $1 "@" want[$1] " (" tag " has " $2 ")"; next }
      }
    }'
}
# `go list -m` prints "<mod> <ver>" or "<mod> <ver> => <mod> <ver>", so $NF is
# the EFFECTIVE version and a replace cannot hide behind the require line.
go list -m $MODS
need="$(go list -m $MODS | awk '{ print $1, $NF }' | below_floor go.mod | awk '{ print $1 }')"
if [ -n "$need" ]; then go get $need; fi
go mod tidy

go build -trimpath -ldflags "-s -w" -o "${{targets.destdir}}/usr/bin/<app>" ./cmd/<app>

# The float is only real if it reached the BINARY. Every `dep`/`=>` line of
# `go version -m` must be at or above its floor -- the `=>` line is what
# catches a replace-shadowed float. Fail the build rather than ship it.
bad="$(go version -m "${{targets.destdir}}/usr/bin/<app>" \
  | awk '$1 == "dep" || $1 == "=>" { print $2, $3 }' | below_floor <app>)"
[ -z "$bad" ] || { echo "❌ 0-CVE float did not reach the binary:"; echo "$bad"; exit 1; }
```

Notes:

- `go list -m` is printed unfiltered first, so the build log always shows what
  MVS actually selected — including any `=> …` redirect.
- The comparison is `>=`, deliberately. A legitimately newer upstream version
  passes; only a version *below* the floor fails. Never assert equality.
- The comparator handles `vMAJOR.MINOR.PATCH`. Pseudo-versions and prerelease
  suffixes compare on their numeric parts only, which is fine for release
  floors — don't put a pseudo-version in `FLOOR`.
- Every module named in `FLOOR` must be in the module graph, or `go list -m`
  fails the build. That is intentional: a floor for a module the app no longer
  uses is dead weight, delete it.
- Vendored builds (`GOFLAGS=-mod=vendor`) must run the float with
  `GOFLAGS=-mod=mod` and re-run `go mod vendor` when `$need` is non-empty;
  otherwise `go build` compiles the stale `vendor/` tree. The binary assertion
  catches that case too.
- Multi-binary recipes: run the assertion for each binary. `go version -m`
  reports only the modules that binary actually links, so a floor for a module
  a given binary does not import is simply not checked.

Live examples: `apps/authelia` (replace dropped), `apps/spicedb` (upstream above
the floor), `apps/tekton` (vendored), `apps/perses` (multi-binary).


## golang.org/x/mod: use v0.40.0, NOT the version Trivy names (2026-08-20)

Trivy reports `golang.org/x/mod` as fixed in **v0.37.0**. Floating to v0.37.0 clears the
Trivy row and still ships a vulnerable module: govulncheck `-mode=binary` on a real binary
flags GO-2026-6180 / GO-2026-6179 against v0.37.0, fixed in v0.40.0. This surfaced on
krakend, where upstream requires v0.36.0 and `go mod tidy` after any other float pulls
v0.37.0 on its own.

So the floor is **v0.40.0**. Two general lessons:

1. **The "fixed" version a scanner names is not always clean.** Same trap as the
   Elasticsearch APM agent, where log4j-api 2.26.0 was the newer version AND was still
   vulnerable (fixes were 2.25.5 / 2.26.1). Check that the version you float TO is itself
   clear, rather than trusting the Fixed Version column.
2. **Trivy and govulncheck disagree, and both are useful.** Trivy reads the module list;
   govulncheck in binary mode reasons about what the binary actually reaches. When they
   differ, take the higher floor. Do not try to prove a module safe with a throwaway
   `main.go` that imports a package without calling it: govulncheck finds nothing because
   nothing is reachable, which looks like a clean result and is not one.

`golang.org/x/mod` enters the graph transitively almost everywhere (any `go mod tidy`
pulls it), so expect this on most Go apps rather than a few.

---

# Bundled dependencies are INVISIBLE to Trivy (2026-08-20)

Trivy finds JavaScript dependencies by reading `package.json` / `node_modules`. An app that
esbuild-bundles everything into a single `.cjs` ships neither, so **Trivy reports it clean
regardless of what is inside the bundle**. `adc` is the first catalog app in that shape: one
4.7 MB `main.cjs`, no manifest, no module tree. Trivy found zero JS packages.
`pnpm audit --prod` on the same build tree found **8 HIGH advisories**.

This is the same failure mode as a statically linked C library. APISIX's `saml-auth` was
dropped this round because it links api7's 2019 xmlsec fork into a static `.a` that Trivy
cannot open. Different language, identical problem: the gate cannot inspect the component,
so a green scan is not evidence.

## The rule

If an app's dependencies do not survive into the image as something Trivy can parse, the
recipe MUST carry its own dependency gate, and that gate must fail the build. For Node, that
is `pnpm audit --prod` (or `npm audit`) as a hard step. A clean Trivy result on a bundled app
means nothing at all.

## Two traps found while doing it on adc

1. **A pnpm `overrides:` entry does NOT apply to a `catalog:` spec.** js-yaml arrived through
   the pnpm catalog, and the override was silently ignored until the catalog entry itself was
   raised. Silently, meaning the advisory stayed and the override looked applied.
2. **An override cannot reach a dependency that a package pre-bundles.** `glob` 13.0.6 points
   its default export at a minified `index.min.js` with minimatch and brace-expansion
   *inlined*. So the brace-expansion actually reaching the artifact was glob's vendored
   pre-5.0.7 copy. `pnpm audit` went quiet because the override fixed the *resolved* package,
   while the vulnerable code stayed in the bundle. 13.0.6 is the newest glob, so there was
   nothing to bump to. Fix: repoint the export at glob's own modular build, which imports the
   real floated packages.

Trap 2 is the one to remember: **the advisory clearing is not proof the code changed.** Verify
against the artifact. On adc that meant grepping the built bundle for a constant that only
exists in the fixed version (`brace-expansion` 5.0.9's `4e6` `EXPANSION_MAX_LENGTH`), plus a
runtime test that exercises the de-vendored path rather than merely inspecting it.

## Consequence for the nightly sweep

A nightly that runs only Trivy will report bundled apps clean forever while their
dependencies rot. Those apps need their own audit step run on a schedule, not just at build
time. Catalog apps in this shape today: `adc`. Node apps worth re-checking for the same
pattern: `ghost`, `excalidraw`, `coolify-realtime`, `xyops`, `apisix` (its dashboard stage).

## Third instance: Lua rocks are invisible too (2026-08-20)

Trivy has no Lua/luarocks analyzer. On the apisix image it reports:

    [wolfi] Detecting vulnerabilities...  pkg_num=40
    Number of language-specific files     num=0

A clean scan there covers 40 Wolfi apks and nothing else. The ~58 Lua rocks and the nginx +
LuaJIT built into apisix-runtime are not looked at.

So the running list of components our gate cannot inspect is:

| shape | example | substitute gate |
|---|---|---|
| statically linked C library | api7's xmlsec1 fork in lua-resty-saml | none -- component dropped |
| bundled/inlined JS | adc's single main.cjs | `pnpm audit --prod` as a hard build step |
| Lua rocks | apisix's luarocks tree | none available; pin versions from upstream's rockspec |

The general rule stands and is worth stating once more: **a green scan is only evidence
about the components the scanner could parse.** Before trusting one, check what it actually
counted. `pkg_num` and `Number of language-specific files` are printed on every run and are
the fastest way to notice that a whole dependency tree was skipped.

Where no substitute gate exists, say so in the recipe rather than letting the tick imply
coverage it does not have.
