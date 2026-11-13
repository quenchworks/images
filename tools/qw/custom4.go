package main

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

func init() {
	for app, c := range map[string]check{
		"authentik": {src: ghRel("goauthentik/authentik"), lines: 2, f: filter{
			keep:  func(t string) bool { return strings.Contains(t, "/") },
			clean: func(t string) string { return t[strings.LastIndex(t, "/")+1:] }}},
		// Codeberg's API first; its git refs when the API is down.
		"forgejo": {src: func() ([]string, error) {
			var rels []map[string]any
			if err := jsonGet("https://codeberg.org/api/v1/repos/forgejo/forgejo/releases?limit=50", &rels); err == nil {
				var tags []string
				for _, r := range rels {
					if p, _ := r["prerelease"].(bool); p {
						continue
					}
					if d, _ := r["draft"].(bool); d {
						continue
					}
					tags = append(tags, strings.TrimLeft(str(r["tag_name"]), "v"))
				}
				return tags, nil
			}
			out, err := exec.Command("git", "ls-remote", "--tags", "https://codeberg.org/forgejo/forgejo.git").Output()
			if err != nil {
				return nil, err
			}
			var tags []string
			for _, m := range regexp.MustCompile(`(?m)refs/tags/v(\d+\.\d+\.\d+)$`).FindAllStringSubmatch(string(out), -1) {
				tags = append(tags, m[1])
			}
			return uniq(tags), nil
		}, lines: 2},
		"varnish": {run: func(w io.Writer) error {
			vs, err := wolfi("varnish")
			if err != nil {
				return err
			}
			reportLines(w, "varnish", vs, 2, filter{})
			tags, err := github("varnish/varnish")
			if err != nil {
				return err
			}
			up := map[string]string{}
			for _, t := range tags {
				v := regexp.MustCompile(`^v?arnish-`).ReplaceAllString(t, "")
				if !startsDigit.MatchString(v) {
					continue
				}
				l := lineKey(v, 2)
				if old, ok := up[l]; !ok || vless(old, v) {
					up[l] = v
				}
			}
			ls := keys(up)
			sortDesc(ls)
			var parts []string
			for _, l := range ls[:min(4, len(ls))] {
				parts = append(parts, l+"->"+up[l])
			}
			fmt.Fprintf(w, "  upstream (github.com/varnish/varnish): %s\n", strings.Join(parts, ", "))
			return nil
		}},
		// the Kafka version marked default in the newest operator release's kafka-versions.yaml
		"strimzi-kafka": {src: func() ([]string, error) {
			tags, err := github("strimzi/strimzi-kafka-operator")
			if err != nil {
				return nil, err
			}
			var ts []string
			for _, t := range tags {
				if startsDigit.MatchString(t) {
					ts = append(ts, t)
				}
			}
			if len(ts) == 0 {
				return nil, nil
			}
			sortDesc(ts)
			newest := ts[0]
			b, err := fetch("https://raw.githubusercontent.com/strimzi/strimzi-kafka-operator/" + newest + "/kafka-versions.yaml")
			if err != nil {
				return nil, err
			}
			var out []string
			for _, block := range strings.Split(string(b), "\n- ") {
				m := regexp.MustCompile(`version:\s*(\S+)`).FindStringSubmatch(block)
				if m != nil && regexp.MustCompile(`(?m)^\s*default:\s*true`).MatchString(block) {
					out = append(out, newest+"-kafka-"+m[1])
				}
			}
			return out, nil
		}, n: 1},
		// kgateway-envoy moves in lockstep with kgateway: same VERSIONS and ENVOY_PKG.
		"kgateway-envoy": {run: func(w io.Writer) error {
			tags, err := github("kgateway-dev/kgateway")
			if err != nil {
				return err
			}
			report(w, "kgateway-envoy", tags, 0, filter{})
			window := func(app string) string {
				out, _ := exec.Command("bash", "-c", "source "+imagesRoot+"/apps/"+app+"/build.conf; echo ${VERSIONS[@]} '|' ${ENVOY_PKG[@]}").Output()
				return strings.Join(strings.Fields(string(out)), " ")
			}
			cp, dp := window("kgateway"), window("kgateway-envoy")
			if cp != dp {
				fmt.Fprintf(w, "  LOCKSTEP BROKEN:    apps/kgateway %s\n                      apps/kgateway-envoy %s\n", pyList(strings.Fields(cp)), pyList(strings.Fields(dp)))
				return nil
			}
			fmt.Fprintf(w, "  lockstep:           matches apps/kgateway (%s)\n", strings.Join(current("kgateway"), " "))
			return nil
		}},
		// CockroachDB has no release feed we trust: probe the binary URL for nearby versions.
		"cockroachdb": {src: func() ([]string, error) {
			cands := map[string]bool{}
			for _, v := range current("cockroachdb") {
				k := vkey(v)
				y, m, p := k[0], k[1], k[2]
				for q := p; q < p+6; q++ {
					cands[fmt.Sprintf("%d.%d.%d", y, m, q)] = true
				}
				for _, d := range []int{1, 2} {
					for q := 0; q < 3; q++ {
						cands[fmt.Sprintf("%d.%d.%d", y, m+d, q)] = true
					}
				}
				for q := 0; q < 3; q++ {
					cands[fmt.Sprintf("%d.1.%d", y+1, q)] = true // YY.1 opens each year
				}
			}
			hc := &http.Client{Timeout: 20 * time.Second}
			var out []string
			for v := range cands {
				r, err := hc.Head("https://binaries.cockroachdb.com/cockroach-v" + v + ".linux-amd64.tgz")
				if err == nil {
					r.Body.Close()
					if r.StatusCode == 200 {
						out = append(out, v)
					}
				}
			}
			sort.Strings(out)
			return out, nil
		}},
	} {
		if _, dup := table[app]; dup {
			panic("check defined twice: " + app)
		}
		table[app] = c
	}
}
