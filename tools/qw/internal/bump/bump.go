package bump

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/quenchworks/images/tools/qw/internal/common"
	"github.com/quenchworks/images/tools/qw/internal/core"
)

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Every value must match its shape: a failed lookup otherwise writes its ERROR BODY into
// the recipe (gh prints a 404 JSON blob on stdout, which is non-empty).
var mapShapes = map[string]*regexp.Regexp{
	"SHA256":  regexp.MustCompile(`^[0-9a-f]{64}$`),
	"SHA512":  regexp.MustCompile(`^[0-9a-f]{128}$`),
	"SHA_AMD": regexp.MustCompile(`^[0-9a-f]{64}([0-9a-f]{64})?$`), // elasticsearch keeps sha512 here
	"SHA_ARM": regexp.MustCompile(`^[0-9a-f]{64}([0-9a-f]{64})?$`),
	"COMMIT":  hex40,
	"PIN":     regexp.MustCompile(`^[\w.+~-]+-r\d+$`),
	"PKG":     regexp.MustCompile(`^[a-z][\w.+-]*$`),
	"SRC_SHA": regexp.MustCompile(`^[0-9a-f]{64}$`),
	"COMMITS": hex40,
}

func readApp(app, file string) string {
	b, _ := os.ReadFile(filepath.Join(core.ImagesRoot, "apps", app, file))
	return string(b)
}

func subVer(u, v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(u, "${{package.version}}", v), "__VER__", v)
}

var uriSha = regexp.MustCompile(`uri:\s*"?(\S+?)"?\s*\n\s*expected-(sha256|sha512):\s*__(\w+)__`)

// uriFor: the uri whose expected-sha is this placeholder. Recipes fetch several things, so
// "the first uri" hashes the wrong artifact for every map but one.
func uriFor(app, v, ph string) (string, string) {
	for _, m := range uriSha.FindAllStringSubmatch(readApp(app, "melange.yaml"), -1) {
		if m[3] == ph {
			u := subVer(m[1], v)
			if strings.Contains(u, "${") || strings.Contains(u, "__") {
				return "", m[2]
			}
			return u, m[2]
		}
	}
	return "", ""
}

// placeholders: map name -> the placeholder render() fills from it (s|__X__|${MAP[$1]}|).
func placeholders(conf string) map[string]string {
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`__(\w+)__[|/]\$\{(\w+)\[\$1\]\}`).FindAllStringSubmatch(conf, -1) {
		if _, ok := out[m[2]]; !ok {
			out[m[2]] = m[1]
		}
	}
	return out
}

func fetchSha(url, algo string) (string, error) {
	var h hash.Hash = sha256.New()
	if algo == "sha512" {
		h = sha512.New()
	}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "curl/8.0") // fastdl.mongodb.org 403s other agents
	// no overall timeout: a big tarball takes minutes; a stalled read still fails below
	c := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 60 * time.Second}}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	n, err := io.Copy(h, resp.Body)
	if err != nil {
		return "", err
	}
	// an empty body hashes to a valid-looking value that pins nothing
	if n == 0 {
		return "", fmt.Errorf("empty response from %s -- asset missing?", url)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// archVars reads the recipe's own `case $(uname -m)` mapping (garch=amd64, esarch=x86_64...).
func archVars(text string) map[string]map[string]string {
	out := map[string]map[string]string{"amd": {}, "arm": {}}
	for _, p := range [][2]string{{"x86_64", "amd"}, {"aarch64", "arm"}} {
		for _, m := range regexp.MustCompile(`(?s)`+p[0]+`\)(.*?);;`).FindAllStringSubmatch(text, -1) {
			for _, kv := range regexp.MustCompile(`(\w+)=([\w.-]+)`).FindAllStringSubmatch(m[1], -1) {
				if _, ok := out[p[1]][kv[1]]; !ok {
					out[p[1]][kv[1]] = kv[2]
				}
			}
		}
	}
	return out
}

// melangeURIs: every fetch URL in the recipe, resolved for one arch; unresolvable ones dropped.
func melangeURIs(app, v, arch string) []string {
	text := readApp(app, "melange.yaml")
	if text == "" {
		return nil
	}
	amap := archVars(text)[arch]
	var out []string
	for _, m := range regexp.MustCompile(`(?:uri|url|binurl|srvurl)[:=]\s*"?(https://[^\s"']+)`).FindAllStringSubmatch(text, -1) {
		u := subVer(m[1], v)
		u = regexp.MustCompile(`\$\{VER\}|\$VER\b`).ReplaceAllString(u, v)
		for k, val := range amap {
			u = strings.ReplaceAll(strings.ReplaceAll(u, "${"+k+"}", val), "$"+k, val)
		}
		if strings.Contains(u, "${") || strings.Contains(u, "__") {
			continue
		}
		out = append(out, u)
	}
	return out
}

var amdToks = []string{"x86_64", "amd64", "x86-64", "x64"}

var armToks = []string{"aarch64", "arm64", "arm_64"}

func perArch(url, arch string) string {
	want, other := amdToks, armToks
	if arch == "arm" {
		want, other = armToks, amdToks
	}
	for _, a := range want {
		for _, b := range other {
			if strings.Contains(url, b) {
				return strings.ReplaceAll(url, b, a)
			}
		}
	}
	return url
}

func ghq(path, jq string) string {
	out, _ := exec.Command("gh", "api", path, "--jq", jq).Output()
	return strings.TrimSpace(string(out))
}

// bareTag finds `tag:` not preceded by a word char or '-' (RE2 has no lookbehind):
// a plain search pinned meilisearch's `dashboard-tag:` commit as the app's.
func bareTag(mel string) string {
	for _, m := range regexp.MustCompile(`tag:\s*(\S+)`).FindAllStringSubmatchIndex(mel, -1) {
		if s := m[0]; s > 0 {
			c := mel[s-1]
			if c == '-' || c == '_' || (c >= '0' && c <= '9') || (c|0x20 >= 'a' && c|0x20 <= 'z') {
				continue
			}
		}
		return mel[m[2]:m[3]]
	}
	return ""
}

func tagCommit(app, v string) string {
	mel := readApp(app, "melange.yaml")
	m := regexp.MustCompile(`repository:\s*https://github\.com/([^\s/]+/[^\s/.]+)`).FindStringSubmatch(mel)
	if m == nil {
		m = regexp.MustCompile(`uri:\s*https://github\.com/([^\s/]+/[^\s/.]+)/archive/`).FindStringSubmatch(mel)
	}
	if m == nil {
		return ""
	}
	repo := m[1]
	tag := bareTag(mel)
	if tag == "" {
		tag = "${{package.version}}"
	}
	tag = strings.Trim(subVer(tag, v), `"`)
	for _, cand := range []string{tag, "v" + v, v} {
		sha := ghq("repos/"+repo+"/git/ref/tags/"+cand, ".object.sha")
		typ := ghq("repos/"+repo+"/git/ref/tags/"+cand, ".object.type")
		if !hex40.MatchString(sha) {
			continue // 404 bodies are non-empty; only a real sha counts
		}
		if typ != "tag" {
			return sha // lightweight tag: already the commit
		}
		if d := ghq("repos/"+repo+"/git/tags/"+sha, ".object.sha"); hex40.MatchString(d) {
			return d
		}
	}
	return ""
}

// pinNames: package names a PIN recipe installs. The value is only the revision; the NAME
// is version-qualified in render() or melange (openldap-2.6, gradle-9).
func pinNames(conf, v, mel string) []string {
	mm := strings.Join(strings.SplitN(v, ".", 3)[:min(2, len(strings.Split(v, ".")))], ".")
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^\s*-\s+([A-Za-z][\w.+-]*)=__\w+__`).FindAllStringSubmatch(mel, -1) {
		names = append(names, m[1])
	}
	for _, m := range regexp.MustCompile(`\|([A-Za-z][\w.+${}%*\\-]*)=\$`).FindAllStringSubmatch(conf, -1) {
		n := strings.ReplaceAll(strings.ReplaceAll(m[1], "${v%.*}", mm), "${v}", v)
		if !strings.ContainsAny(n, "${") {
			names = append(names, n)
		}
	}
	return names
}

func wolfiPin(pkg, v string) string {
	idx, err := core.LoadWolfi()
	if err != nil {
		return ""
	}
	best, bestRev := "", -1
	for _, full := range idx[pkg] {
		base, rev, ok := strings.Cut(full, "-r")
		if !ok || base != v {
			continue
		}
		if r, _ := strconv.Atoi(rev); r > bestRev {
			best, bestRev = full, r
		}
	}
	return best
}

// chartLineDropped: the chart's appVersion when no given version is on its major.minor line
// (rqlite, ollama and victorialogs were stranded that way on 2026-10-04).
func chartLineDropped(app string, versions []string) string {
	b, err := os.ReadFile(filepath.Join(core.ImagesRoot, "..", "charts", "quench", app, "Chart.yaml"))
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`(?m)^appVersion:\s*"?v?([0-9][^"\s]*)"?\s*$`).FindStringSubmatch(string(b))
	if m == nil {
		return ""
	}
	p := strings.Split(m[1], ".")
	line := strings.Join(p[:min(2, len(p))], ".") + "."
	for _, v := range versions {
		if strings.HasPrefix(strings.TrimLeft(v, "v")+".", line) {
			return ""
		}
	}
	return m[1]
}

func Run(args []string) int {
	if len(args) < 2 {
		fmt.Println("usage: qw bump <app> <version> [<version> ...]")
		return 2
	}
	app, versions := args[0], args[1:]
	if p := chartLineDropped(app, versions); p != "" {
		fmt.Printf("!! %s: the chart pins %s and no version given is on its line; pass %s (or a newer patch of it) too, or move the chart first\n", app, p, p)
		return 4
	}
	confPath := filepath.Join(core.ImagesRoot, "apps", app, "build.conf")
	cb, err := os.ReadFile(confPath)
	if err != nil {
		fmt.Println(err)
		return 2
	}
	text := string(cb)
	mel := readApp(app, "melange.yaml")
	var maps []string
	for _, m := range regexp.MustCompile(`declare -A (\w+)=\(`).FindAllStringSubmatch(text, -1) {
		maps = append(maps, m[1])
	}
	known := map[string]bool{"SHA256": true, "SHA512": true, "SHA_AMD": true, "SHA_ARM": true, "COMMIT": true, "COMMITS": true, "PIN": true, "PKG": true, "SRC_SHA": true}
	// any other map render() puts in a placeholder the recipe pairs with a fetch uri
	phs := placeholders(text)
	generic := map[string]string{}
	var unknown []string
	for _, m := range maps {
		if known[m] {
			continue
		}
		if ph, ok := phs[m]; ok {
			if _, algo := uriFor(app, "0", ph); algo != "" {
				generic[m] = ph
				continue
			}
		}
		unknown = append(unknown, m)
	}
	if len(unknown) > 0 {
		fmt.Printf("!! %s: unhandled map(s) %s -- bump by hand\n", app, common.PyList(unknown))
		return 3
	}
	entries := map[string]map[string]string{}
	for _, m := range maps {
		entries[m] = map[string]string{}
	}
	fail := func(f string, a ...any) int { fmt.Printf("!! %s: "+f+"\n", append([]any{app}, a...)...); return 3 }
	shaAlgo := "sha256"
	if strings.Contains(mel, "sha512sum -c") {
		shaAlgo = "sha512" // per recipe, not per map name (elasticsearch)
	}
	for _, v := range versions {
		uris := melangeURIs(app, v, "amd")
		for _, m := range maps {
			var val string
			switch {
			case m == "SRC_SHA":
				u, _ := uriFor(app, v, "SRC_SHA")
				if u == "" {
					return fail("no uri paired with __SRC_SHA__ for %s", v)
				}
				if val, err = fetchSha(u, "sha256"); err != nil {
					return fail("%v", err)
				}
			case m == "SHA256" || m == "SHA512":
				algo := strings.ToLower(strings.Replace(m, "SHA", "sha", 1))
				u, _ := uriFor(app, v, m)
				if u == "" {
					if len(uris) == 0 {
						return fail("no resolvable uri for %s", v)
					}
					u = uris[0]
				}
				if val, err = fetchSha(u, algo); err != nil {
					return fail("%v", err)
				}
			case m == "SHA_AMD" || m == "SHA_ARM":
				if len(uris) == 0 {
					return fail("no resolvable uri for %s", v)
				}
				a, toks := "amd", amdToks[:0:0]
				toks = []string{"amd64", "x86_64", "x86-64", "x64"}
				if m == "SHA_ARM" {
					a, toks = "arm", []string{"arm64", "aarch64", "arm_64"}
				}
				au := melangeURIs(app, v, a)
				if len(au) == 0 {
					au = []string{perArch(uris[0], a)}
				}
				// a recipe fetching source AND binaries lists the source first: prefer the
				// url carrying this arch's token
				picked := au[0]
				for _, u := range au {
					if common.ContainsAny(u, toks) {
						picked = u
						break
					}
				}
				if val, err = fetchSha(picked, shaAlgo); err != nil {
					return fail("%v", err)
				}
			case generic[m] != "":
				u, algo := uriFor(app, v, generic[m])
				if u == "" {
					return fail("no resolvable uri for __%s__ at %s (it needs another map's value) -- bump by hand", generic[m], v)
				}
				if val, err = fetchSha(u, algo); err != nil {
					return fail("%v", err)
				}
			case m == "COMMIT" || m == "COMMITS":
				if val = tagCommit(app, v); val == "" {
					return fail("could not resolve tag->commit for %s", v)
				}
			case m == "PIN":
				cands := pinNames(text, v, mel)
				if len(cands) == 0 {
					cands = []string{app}
				}
				for _, n := range cands {
					if val = wolfiPin(n, v); val != "" {
						break
					}
				}
				if val == "" {
					return fail("none of %s at %s found in the Wolfi index", common.PyList(cands), v)
				}
			case m == "PKG":
				// carry the existing name forward on this version's line: go-1.26 takes the
				// line, erlang-27 takes the major (2026-10-19)
				prev := regexp.MustCompile(`declare -A PKG=\(\s*\n\s*\[[^\]]+\]=(\S+)`).FindStringSubmatch(text)
				if prev == nil {
					return fail("cannot infer the PKG naming pattern")
				}
				p := strings.Split(v, ".")
				if regexp.MustCompile(`\d+\.\d+$`).MatchString(prev[1]) {
					val = regexp.MustCompile(`\d+\.\d+$`).ReplaceAllString(prev[1], strings.Join(p[:min(2, len(p))], "."))
				} else {
					val = regexp.MustCompile(`\d+$`).ReplaceAllString(prev[1], p[0])
				}
			}
			entries[m][v] = val
		}
	}
	text = regexp.MustCompile(`(?m)^VERSIONS=\(.*\)$`).ReplaceAllLiteralString(text, "VERSIONS=("+strings.Join(versions, " ")+")")
	// a recipe another app renders (trivy-scanner renders trivy) keeps its other entries
	borrowed := false
	dirs, _ := os.ReadDir(filepath.Join(core.ImagesRoot, "apps"))
	for _, d := range dirs {
		if d.Name() == app {
			continue
		}
		for _, f := range []string{"build.conf", "melange.yaml"} {
			if strings.Contains(readApp(d.Name(), f), "apps/"+app+"/") {
				borrowed = true
			}
		}
	}
	for _, m := range maps {
		keys := append([]string(nil), versions...)
		if borrowed {
			if old := regexp.MustCompile(`declare -A ` + m + `=\(([^)]*)\)`).FindStringSubmatch(text); old != nil {
				for _, kv := range regexp.MustCompile(`\[([^\]]+)\]=(\S+)`).FindAllStringSubmatch(old[1], -1) {
					if _, ok := entries[m][kv[1]]; !ok {
						entries[m][kv[1]] = kv[2]
						keys = append(keys, kv[1])
					}
				}
			}
			sort.SliceStable(keys, func(i, j int) bool { return lessMixed(keys[i], keys[j]) })
		}
		var body []string
		for _, k := range keys {
			body = append(body, "  ["+k+"]="+entries[m][k])
		}
		block := "declare -A " + m + "=(\n" + strings.Join(body, "\n") + "\n)"
		// maps come in BOTH layouts; rewriting only one moves VERSIONS and leaves the map
		// stale, which dies later on `PIN[$1]: unbound variable`
		multi := regexp.MustCompile(`declare -A ` + m + `=\(\n(?:\s*\[[^\]]+\]=\S*\n)*\)`)
		single := regexp.MustCompile(`declare -A ` + m + `=\((?:\s*\[[^\]]+\]=\S*)+\s*\)`)
		switch {
		case multi.MatchString(text):
			text = replaceFirst(multi, text, block)
		case single.MatchString(text):
			text = replaceFirst(single, text, block)
		default:
			return fail("could not locate the %s map to rewrite", m)
		}
	}
	for _, m := range maps {
		for v, val := range entries[m] {
			shape := mapShapes[m]
			if shape == nil {
				shape = mapShapes["SHA256"]
				if _, algo := uriFor(app, "0", generic[m]); algo == "sha512" {
					shape = mapShapes["SHA512"]
				}
			}
			if !shape.MatchString(val) {
				return fail("refusing to write %s[%s]=%s -- wrong shape", m, v, common.PyRepr(val[:min(60, len(val))]))
			}
		}
	}
	if err := os.WriteFile(confPath, []byte(text), 0o644); err != nil {
		fmt.Println(err)
		return 2
	}
	tail := ""
	if len(maps) > 0 {
		tail = "  [" + strings.Join(maps, ", ") + "]"
	}
	fmt.Printf("ok %s -> %s%s\n", app, strings.Join(versions, " "), tail)
	return 0
}

func replaceFirst(re *regexp.Regexp, s, repl string) string {
	loc := re.FindStringIndex(s)
	return s[:loc[0]] + repl + s[loc[1]:]
}

// lessMixed orders keys split on . and -: numbers before words, numbers numerically.
func lessMixed(a, b string) bool {
	sp := regexp.MustCompile(`[.-]`)
	pa, pb := sp.Split(a, -1), sp.Split(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil && na != nb:
			return na < nb
		case ea == nil && eb != nil:
			return true
		case ea != nil && eb == nil:
			return false
		case ea != nil && eb != nil && pa[i] != pb[i]:
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}
