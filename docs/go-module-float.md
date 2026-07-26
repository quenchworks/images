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
FLOOR="golang.org/x/net@v0.56.0 golang.org/x/text@v0.39.0 google.golang.org/grpc@v1.82.1"
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
