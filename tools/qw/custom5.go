package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	for app, c := range map[string]check{
		// Wolfi ships elixir per Erlang major (erl28-elixir-1.19); compare per elixir line.
		"elixir": {run: func(w io.Writer) error {
			m, err := wolfiMatch(`^erl\d+-elixir-\d+\.\d+$`)
			if err != nil {
				return err
			}
			lines := map[string]string{}
			for _, v := range m {
				l := lineKey(v, 2)
				if old, ok := lines[l]; !ok || vless(old, v) {
					lines[l] = v
				}
			}
			fmt.Fprintln(w, "elixir")
			var behind []string
			for _, c := range current("elixir") {
				l := lineKey(c, 2)
				v, ok := lines[l]
				upd := ok && vless(c, v)
				if upd {
					behind = append(behind, v)
				}
				if !ok {
					v = "?"
				}
				fmt.Fprintf(w, "  elixir-%s: have=%10s  latest=%10s  %s\n", l, c, v, okOr(upd))
			}
			fmt.Fprintf(w, "  => %s\n", updOr(behind))
			return nil
		}},
		// 22 is built from the nodejs.org tarball (Wolfi moved nodejs-22 to enterprise,
		// 2026-08-14); 24 and 26 come from Wolfi.
		"node": {run: func(w io.Writer) error {
			cur := map[string]string{}
			for _, v := range current("node") {
				cur[strings.SplitN(v, ".", 2)[0]] = v
			}
			fmt.Fprintln(w, "node")
			var behind []string
			for _, maj := range []string{"22", "24", "26"} {
				var cands []string
				var err error
				if maj == "22" {
					var idx []map[string]any
					err = jsonGet("https://nodejs.org/dist/index.json", &idx)
					for _, r := range idx {
						if v := str(r["version"]); strings.HasPrefix(v, "v22.") {
							cands = append(cands, v[1:])
						}
					}
				} else {
					cands, err = wolfi("nodejs-" + maj)
				}
				if err != nil {
					return err
				}
				sortDesc(cands)
				newest := cands[0]
				have, ok := cur[maj]
				if !ok {
					have = "?"
				}
				upd := !ok || vless(have, newest)
				if upd {
					behind = append(behind, newest)
				}
				fmt.Fprintf(w, "  nodejs-%s: have=%10s  latest=%10s  %s\n", maj, have, newest, okOr(upd))
			}
			fmt.Fprintf(w, "  => %s\n", updOr(behind))
			return nil
		}},
		// step-ca ships the step CLI too, pinned separately in STEP.
		"step-ca": {run: func(w io.Writer) error {
			tags, err := github("smallstep/certificates")
			if err != nil {
				return err
			}
			report(w, "step-ca", tags, 1, filter{})
			out, _ := exec.Command("bash", "-c", "source "+imagesRoot+"/apps/step-ca/build.conf; echo ${STEP[@]}").Output()
			step := strings.Fields(string(out))
			cli, err := github("smallstep/cli")
			if err != nil {
				return err
			}
			var cands []string
			for _, t := range cli {
				if startsDigit.MatchString(t) {
					cands = append(cands, t)
				}
			}
			sortDesc(cands)
			have, newest := "", ""
			if len(step) > 0 {
				sortDesc(step)
				have = step[0]
			}
			if len(cands) > 0 {
				newest = cands[0]
			}
			var status string
			switch {
			case newest == "":
				status = "BROKEN CHECK -- smallstep/cli returned NO candidates"
			case have == "":
				status = "BROKEN CHECK -- no STEP pin found in build.conf"
			case vless(have, newest):
				status = "UPDATE -> ['" + newest + "']"
			default:
				status = "ok"
			}
			h, l := have, "[]"
			if h == "" {
				h = "?"
			}
			if newest != "" {
				l = pyList([]string{newest})
			}
			fmt.Fprintf(w, "%-20s have=%14s  latest1=%s  %s\n", "step-ca (step cli)", h, l, status)
			return nil
		}},
		// NGF pins its nginx, njs and agent per release: shown, not bumped on their own.
		"nginx-gateway-fabric-nginx": {run: func(w io.Writer) error {
			tags, err := github("nginx/nginx-gateway-fabric")
			if err != nil {
				return err
			}
			const app = "nginx-gateway-fabric-nginx"
			report(w, app, tags, 0, filter{})
			ver := "?"
			if cur := current(app); len(cur) > 0 {
				ver = cur[len(cur)-1]
			}
			b, _ := os.ReadFile(filepath.Join(imagesRoot, "apps", app, "build.conf"))
			pinned := func(name string) string {
				m := regexp.MustCompile(`declare -A ` + name + `=\(\s*\[` + regexp.QuoteMeta(ver) + `\]=(\S+)`).FindStringSubmatch(string(b))
				if m == nil {
					return "?"
				}
				return m[1]
			}
			newest := func(cands []string) string {
				best := "?"
				for _, c := range cands {
					if full(`\d+\.\d+\.\d+`)(c) && (best == "?" || vless(best, c)) {
						best = c
					}
				}
				return best
			}
			nginx, err := githubTags("nginx/nginx")
			if err != nil {
				return err
			}
			for i := range nginx {
				nginx[i] = strings.ReplaceAll(nginx[i], "release-", "")
			}
			njs, err := githubTags("nginx/njs")
			if err != nil {
				return err
			}
			agent, err := github("nginx/agent")
			if err != nil {
				return err
			}
			var agent3 []string
			for _, t := range agent {
				if strings.HasPrefix(t, "3.") {
					agent3 = append(agent3, t)
				}
			}
			fmt.Fprintf(w, "  components pinned by NGF %s (bump only with the release):\n", ver)
			for _, r := range [][3]string{
				{"nginx", pinned("NGINX_VER"), newest(nginx)},
				{"njs", pinned("NJS_VER"), newest(njs)},
				{"nginx-agent", pinned("AGENT_VER"), newest(agent3)},
			} {
				state := "ok"
				if r[1] != r[2] {
					state = "upstream moved"
				}
				fmt.Fprintf(w, "    %-12s have=%10s  upstream-newest=%10s  %s\n", r[0], r[1], r[2], state)
			}
			return nil
		}},
	} {
		if _, dup := table[app]; dup {
			panic("check defined twice: " + app)
		}
		table[app] = c
	}
}

func okOr(upd bool) string {
	if upd {
		return "UPDATE"
	}
	return "ok"
}

func updOr(behind []string) string {
	if len(behind) > 0 {
		return "UPDATE -> " + pyList(behind)
	}
	return "ok"
}
