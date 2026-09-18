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

## 4. A floor is a version comparison, and a backport is not

`below_floor` asks "is the installed version higher than the floor". That is the
wrong question whenever upstream backports a fix across several release lines.

harbor-trivy-adapter floored `containerd/v2` at `v2.2.8` and still shipped
CVE-2026-53495. The graph resolved **v2.3.3**, which clears a v2.2.8 floor. That
CVE is fixed per line, in 2.0.12, 2.2.8 **and** 2.3.5, so v2.3.3 sits above the
floor and below its own line's fix. The float assertion agreed with the floor and
passed, because it was asked the same wrong question (2026-09-17).

Set the floor to the fix on the **highest line the graph can reach**, not the
lowest fix the advisory lists. And grep the sibling recipes first: eight other
FLOOR entries in this catalog already said `v2.3.5`.

## 5. A floor below what the graph already resolves is a no-op

telegraf's grpc fix shipped as 1.82.2 and 1.83.2, but rclone requires a master
pseudo-version (`v1.84.0-dev…`), so both released fixes sit *below* what is
already in the build list. Flooring to either reads correctly and changes
nothing. The floor had to be the pseudo-version on that branch, which is the
third version Trivy names (2026-09-17).

Check the resolved version before choosing a floor, not just the advisory.

## 6. Quoting: an entry outside the closing quote is not in FLOOR

```sh
FLOOR="golang.org/x/net@v0.58.0 … grpc@v1.83.2" software.sslmate.com/src/go-pkcs12@v0.7.2
```

telegraf carried that for 17 days. The shell assigned FLOOR **without** the last
entry and then tried to run the module path as a command: exit 127, one line of
output, every build failing, and GHSA-mpwr-8vm7-h73f never floored even though
the comment above it said it was. `bash -n` does not catch this; the line is
valid shell (2026-09-17).

Assert the list is whole before using it, rather than trusting the quoting:

```sh
for m in golang.org/x/net google.golang.org/grpc software.sslmate.com/src/go-pkcs12; do
  case " $FLOOR " in *" $m@"*) ;; *) echo "FLOOR is missing $m"; exit 1 ;; esac
done
```

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

## Gotcha: pushing right after a visibility flip can report the repo "disabled" (2026-08-20)

    gh repo edit quenchworks/images --visibility public ...
    git push origin main
    ERROR: Repository 'quenchworks/images' is disabled.
            Please ask the owner to check their account.

That message reads like a billing or abuse suspension, and it is neither. The visibility
change is not instantaneous, and a push landing inside that window gets refused with a
message about the ACCOUNT rather than about timing. A plain retry a few seconds later
succeeds, and workflow_dispatch calls issued in the same window still queue and run.

So: after flipping visibility, retry the push once before concluding anything about the
account. Do not go looking at billing, and do not assume the campaign has been throttled.

## `cmd | grep -q` under `set -o pipefail` fails when the pattern MATCHES (2026-08-20)

`grep -q` exits the instant it finds a match. If the writer is still producing output it
takes SIGPIPE and exits 141, and `pipefail` makes that the pipeline's status. So:

    cmd | grep -q PATTERN || { echo "not found"; exit 1; }   # fires ON SUCCESS
    if cmd | grep -q PATTERN; then ...                       # takes the WRONG branch

Both forms are wrong; the first is loud and the second is silent. It is size- and
timing-dependent, which is what makes it nasty: the same line passes for a page of `--help`
output and fails for the `strings` dump of a 190MB binary.

Seen twice in `apps/argocd/test.sh` on the same day. The UI greps were fixed and the
`ARGOCD_BINARY_NAME` check three lines above was missed, so it reported "override does not
select the server" for an image where the override demonstrably works.

**Fix:** capture, then match. `out="$(cmd)"` then `grep -q PATTERN <<<"$out"`. No pipe, no
SIGPIPE, and the captured output is available to print on failure.

**Scale, measured rather than guessed:** 330 `| grep -q` sites across the test scripts under
pipefail. 193 are `echo "$var" | grep -q`, which is safe -- echo of a small variable finishes
before grep can leave. **137 have a command on the left** (docker run, docker logs, curl,
strings) and are the real exposure.

### Correction: the "echo is safe" split was wrong (same day)

The 193/137 split above claimed `echo "$var" | grep -q` is safe because a small echo
finishes before grep can leave. That reasoning is right and the conclusion is wrong, because
the variable is not always small. temporal's test did:

    logs="$(docker logs "$NAME" 2>&1 || true)"
    if echo "$logs" | grep -qiE 'frontend started|...'; then

`$logs` is temporal's entire log output, thousands of JSON lines. echo was still writing,
grep left at the first match, and under pipefail the `if` condition read FALSE **because the
pattern matched**. The poll loop then ran all 90 iterations and timed out against a
perfectly healthy server, emitting `echo: write error: Broken pipe` 90 times as its only
clue.

So the real split is not echo-vs-command, it is small-vs-large output, and that cannot be
determined by reading the line. Re-counted on the actual dangerous shape (left side is
`docker logs`, `strings`, `find`, `cat`, or a variable named logs/names/out/body/dump):
**115 sites**.

### Do NOT try to fix these in bulk

A mechanical rewrite of `A | grep -qX P` to `grep -qX P <<<"$(A)"` looks safe and is not.
Applied across 28 files it produced:

    grep -qi "Usage:" || { echo "..."; exit 1; } <<<"$(echo "$out")"

The here-string binds to the `{ ... }` group, not to grep, so grep reads nothing and the
assertion is silently inverted. `bash -n` passes: it is valid shell and wrong shell. Reverted.

Fix these ONE AT A TIME, by hand, where a failure actually points at one, and re-run the
test afterwards to confirm the assertion still passes for the right reason.

## 7. The toolchain line, not just the `go` line

`GOTOOLCHAIN=local` in these recipes means the installed Wolfi toolchain is the
only one that will ever run. That makes two separate things in go.mod matter,
and only one of them is obvious.

The `go` line is the language version floor. A pin below it fails loudly:

    go: go.mod requires go >= 1.27.1 (running go 1.26.8; GOTOOLCHAIN=local)

The `toolchain` line is what upstream builds with, and `GOTOOLCHAIN=local`
ignores it. That is usually fine. It stops being fine when a dependency guards
code with a build tag, because a build tag excluded by an older toolchain fails
as an undefined symbol in YOUR code, with nothing naming a version at all:

    internal/handlers/handler_register_webauthn.go:101:28:
      undefined: webauthncose.AlgMLDSA44

authelia 4.39.22 reads `go 1.26.0` / `toolchain go1.27.1`. Its go-webauthn bump
to v0.18.0 put ML-DSA behind `//go:build go1.27`, so under go-1.26 those
constants do not exist and the caller does not compile. The `go` line said
1.26.0 and was satisfied. Nothing in the error mentions a Go version.

The check that would have caught it, run before dispatch rather than after:

    curl -fsSL "https://raw.githubusercontent.com/$REPO/v$VER/go.mod" \
      | awk '/^go |^toolchain /'

Take the HIGHER of the two as the toolchain to pin. The `go` line alone is a
floor on the language, not on what the dependency tree can compile.

## 8. It can TIE — two pseudo-versions sharing a numeric triple

`below_floor` compares the first three numeric components and stops. That is
enough for release versions, and blind for pseudo-versions:

    telegraf 1.40.0 requires  google.golang.org/grpc v1.85.0-dev
    the FLOOR reads           google.golang.org/grpc v1.85.0-dev.0.20260825072537-93e31b48545e

Both split to (1, 85, 0). The loop finds no component greater and none smaller,
falls off the end, and prints nothing. So the float did not fire AND the
shipped-binary assertion, which calls the same function, did not fail. The
image scanned as 1 HIGH (CVE-2026-84445) while every check reported clean.

This is the failure mode from `mem:inert-pin-failure-class` in a new disguise:
the floor is correct, the comparison is what is broken.

Once the triple ties, compare the prerelease suffix the way Go does: a release
outranks any prerelease, and otherwise dot-separated identifiers compare left to
right with a prefix ranking lower.

    function pre(v,  p) { p = index(v, "-"); return p ? substr(v, p + 1) : "" }
    function precmp(a, b,  na, nb, x, y, i) {
      if (a == b) return 0
      if (a == "") return 1
      if (b == "") return -1
      na = split(a, x, "."); nb = split(b, y, ".")
      for (i = 1; i <= na && i <= nb; i++) {
        if (x[i] == y[i]) continue
        if (x[i] + 0 != 0 && y[i] + 0 != 0) return (x[i] + 0 < y[i] + 0) ? -1 : 1
        return (x[i] < y[i]) ? -1 : 1
      }
      return (na < nb) ? -1 : 1
    }

then, after the three-component loop:

    if (precmp(pre($2), pre(want[$1])) < 0) print $1 "@" want[$1] " (" tag " has " $2 ")"

Only a floor whose version carries a `-` can tie, so that is the set to sweep:

    grep -ho 'FLOOR="[^"]*"' apps/*/melange.yaml | tr ' ' '\n' | grep '@v.*-'

On 2026-09-18 that was two apps, telegraf and seaweedfs, both on the same grpc
floor and both shipping the CVE.
