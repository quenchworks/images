package check

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"github.com/quenchworks/images/tools/qw/internal/common"
	"github.com/quenchworks/images/tools/qw/internal/core"
)

// Helpers behind the hand-written rows in check_table.go.

const calicoNode = `^calico-node-\d+\.\d+$`

var endoflifeLatest = func(d any) []string {
	return common.Each(d, func(c any) string { return common.Str(common.Path(c, "latest")) })
}

var graviteeCheck = check{src: jsonList("https://hub.docker.com/v2/repositories/graviteeio/apim-gateway/tags?page_size=100",
	func(d any) []string {
		return common.Each(common.Path(d, "results"), func(t any) string { return common.Str(common.Path(t, "name")) })
	}),
	lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}}

var linkerdEdge = check{src: ghRel("linkerd/linkerd2"), lines: 2, f: filter{keep: prefix("edge-"), clean: trim("edge-")}}

// pdnsRefs: PowerDNS tags auth-X.Y.Z / rec-X.Y.Z, read through the matching-refs API.
func pdnsRefs(p string) check {
	return check{src: func() ([]string, error) {
		return core.Gh("repos/PowerDNS/pdns/git/matching-refs/tags/"+p, ".[].ref")
	}, lines: 2, f: group(`refs/tags/` + p + `(\d+\.\d+\.\d+)`)}
}

func uniq(vs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range vs {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// n8nStable: stable n8n versions on or below the line of the npm "stable" dist-tag.
func n8nStable() ([]string, error) {
	var d struct {
		DistTags map[string]string         `json:"dist-tags"`
		Versions map[string]map[string]any `json:"versions"`
	}
	if err := core.JsonGet("https://registry.npmjs.org/n8n", &d); err != nil {
		return nil, err
	}
	line := core.Vkey(core.LineKey(d.DistTags["stable"], 2))
	var out []string
	for v := range d.Versions {
		if !strings.Contains(v, "-") && core.CmpKey(core.Vkey(core.LineKey(v, 2)), line) <= 0 {
			out = append(out, v)
		}
	}
	return out, nil
}

// zabbixSources: every X.Y.Z tarball on the 6.0+ lines of the stable sources tree.
func zabbixSources() ([]string, error) {
	const base = "https://cdn.zabbix.com/zabbix/sources/stable/"
	ls, err := core.Scrape(base, `href="(\d+\.\d+)/"`)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range ls {
		if core.Vkey(l)[0] < 6 {
			continue
		}
		vs, err := core.Scrape(base+l+"/", `zabbix-(\d+\.\d+\.\d+)\.tar\.gz`)
		if err != nil {
			return nil, err
		}
		out = append(out, vs...)
	}
	return out, nil
}

func okOr(upd bool) string {
	if upd {
		return "UPDATE"
	}
	return "ok"
}

func updOr(behind []string) string {
	if len(behind) > 0 {
		return "UPDATE -> " + common.PyList(behind)
	}
	return "ok"
}

// bashVar echoes a bash expression after sourcing an app's build.conf.
func bashVar(app, expr string) string {
	out, _ := exec.Command("bash", "-c", "source "+core.ImagesRoot+"/apps/"+app+"/build.conf; echo "+expr).Output()
	return strings.TrimSpace(string(out))
}

func sortedAsc(vs []string) []string {
	out := slices.Clone(vs)
	core.SortDesc(out)
	slices.Reverse(out)
	return out
}

// linkerd-proxy is paired with each control-plane edge release through its .proxy-version.
func linkerdProxy(w io.Writer) error {
	cp := sortedAsc(core.Current("linkerd-control-plane"))
	ours := sortedAsc(core.Current("linkerd-proxy"))
	var behind, errs []string
	fmt.Fprintln(w, "linkerd-proxy")
	for i, cpv := range cp {
		want := ""
		vs, err := core.Scrape("https://raw.githubusercontent.com/linkerd/linkerd2/edge-"+cpv+"/.proxy-version", `v?(\d+\.\d+\.\d+)`)
		switch {
		case err != nil:
			errs = append(errs, cpv+": "+err.Error())
		case len(vs) == 0:
			errs = append(errs, cpv+": list index out of range")
		default:
			want = vs[0]
		}
		have := "-"
		if i < len(ours) {
			have = ours[i]
		}
		upd := want != "" && want != have
		if upd {
			behind = append(behind, want)
		}
		mark, shown := "ok", want
		if want == "" {
			mark, shown = "?", "?"
		} else if upd {
			mark = "UPDATE"
		}
		fmt.Fprintf(w, "  edge-%s: have=%10s  paired=%10s  %s\n", cpv, have, shown, mark)
	}
	rel, err := core.Github("linkerd/linkerd2-proxy")
	if err != nil {
		return err
	}
	var tags []string
	for _, t := range rel {
		if strings.HasPrefix(t, "release/") {
			tags = append(tags, strings.TrimLeft(strings.TrimPrefix(t, "release/"), "v"))
		}
	}
	if len(tags) > 0 {
		core.SortDesc(tags)
		fmt.Fprintf(w, "  newest upstream proxy release (not yet paired): %s\n", tags[0])
	}
	switch {
	case len(errs) > 0:
		fmt.Fprintf(w, "  => BROKEN CHECK -- could not read .proxy-version: %s\n", common.PyList(errs))
	case len(ours) != len(cp):
		fmt.Fprintf(w, "  => BROKEN CHECK -- %d proxy pins vs %d control-plane pins\n", len(ours), len(cp))
	default:
		fmt.Fprintf(w, "  => %s\n", updOr(behind))
	}
	return nil
}

// adc is the sidecar the ingress controller pins (Makefile ADC_VERSION); it moves with the controller.
func adcCheck(w io.Writer) error {
	const ctlApp = "apisix-ingress-controller"
	ctl := core.Current(ctlApp)
	if len(ctl) == 0 {
		return fmt.Errorf("adc: cannot read apps/%s/build.conf VERSIONS -- broken check", ctlApp)
	}
	ctlVer := ctl[len(ctl)-1]
	if paired := bashVar("adc", "${PAIRED_CONTROLLER:-}"); paired != ctlVer {
		return fmt.Errorf("adc: build.conf PAIRED_CONTROLLER=%q but we ship %s %s -- the sidecar/controller pair has drifted", paired, ctlApp, ctlVer)
	}
	url := "https://raw.githubusercontent.com/apache/" + ctlApp + "/" + ctlVer + "/Makefile"
	mk, err := core.Fetch(url)
	if err != nil {
		return fmt.Errorf("adc: cannot fetch %s: %v -- broken check", url, err)
	}
	m := regexp.MustCompile(`(?m)^ADC_VERSION\s*\?=\s*([0-9][^\s#]*)`).FindStringSubmatch(string(mk))
	if m == nil {
		return fmt.Errorf("adc: no ADC_VERSION in %s %s Makefile -- the pin moved", ctlApp, ctlVer)
	}
	pinned := m[1]
	fmt.Fprintf(w, "  pinned by %s %s (Makefile ADC_VERSION): %s\n", ctlApp, ctlVer, pinned)
	tags, err := core.GithubTags("api7/adc")
	if err != nil {
		return err
	}
	var vs []string
	for _, t := range tags {
		if full(`\d+\.\d+\.\d+`)(t) {
			vs = append(vs, t)
		}
	}
	if len(vs) > 0 {
		core.SortDesc(vs)
		note := "same"
		if vs[0] != pinned {
			note = "AHEAD of the pin -- needs a controller bump too"
		}
		fmt.Fprintf(w, "  api7/adc newest tag (informational): %s (%s)\n", vs[0], note)
	}
	report(w, "adc", []string{pinned}, 1, filter{})
	return nil
}

func contourEnvoyPin(tag string) (string, string) {
	b, err := core.Fetch("https://raw.githubusercontent.com/projectcontour/contour/v" + tag + "/examples/contour/03-envoy.yaml")
	if err != nil {
		return "", fmt.Sprintf("could not fetch 03-envoy.yaml (%v)", err)
	}
	if m := regexp.MustCompile(`image:\s*docker\.io/envoyproxy/envoy:(?:distroless-)?v([0-9.]+)`).FindSubmatch(b); m != nil {
		return string(m[1]), ""
	}
	return "", "no envoyproxy/envoy image in 03-envoy.yaml"
}

func kgatewayEnvoyPin(tag string) (string, string) {
	b, err := core.Fetch("https://raw.githubusercontent.com/kgateway-dev/kgateway/v" + tag + "/Makefile")
	if err != nil {
		return "", fmt.Sprintf("could not fetch the Makefile (%v)", err)
	}
	if m := regexp.MustCompile(`(?m)^export ENVOY_IMAGE \??=\s*\S*envoy:v?([0-9.]+)`).FindSubmatch(b); m != nil {
		return string(m[1]), ""
	}
	if regexp.MustCompile(`(?m)^export ENVOY_IMAGE_AMD64\s*=\s*\S*envoy-gloo`).Match(b) {
		return "", "pins the solo-io envoy-gloo fork on amd64 -- not shippable here"
	}
	return "", "no ENVOY_IMAGE in the Makefile"
}

// envoyPaired reports the app, then whether our envoy image can serve the Envoy its candidate deploys.
func envoyPaired(app, repo string, pinOf func(string) (string, string)) func(io.Writer) error {
	return func(w io.Writer) error {
		tags, err := core.Github(repo)
		if err != nil {
			return err
		}
		behind := report(w, app, tags, 0, filter{})
		pool := behind
		if len(pool) == 0 {
			pool = core.Current(app)
		}
		cand := pool[len(pool)-1]
		pin, perr := pinOf(cand)
		if perr != "" {
			blocked := "UNKNOWN"
			if app == "kgateway" {
				blocked = "UNKNOWN/BLOCKED"
			}
			fmt.Fprintf(w, "  envoy pairing:      %s %s -> %s (%s)\n", app, cand, blocked, perr)
			return nil
		}
		lines := map[string]string{}
		for _, v := range core.Current("envoy") {
			lines[strings.Join(strings.SplitN(v, ".", 3)[:2], ".")] = v
		}
		line := strings.Join(strings.SplitN(pin, ".", 3)[:2], ".")
		ours, ok := lines[line]
		switch {
		case !ok:
			fmt.Fprintf(w, "  envoy pairing:      %s %s wants envoy %s; we ship %s -- NO %s LINE, cannot pair (do not bump)\n",
				app, cand, pin, common.PyList(sortedAsc(common.Keys(lines))), line)
		case core.Vless(ours, pin):
			fmt.Fprintf(w, "  envoy pairing:      %s %s wants envoy %s; we ship %s (same %s line, ours is older) -- pairable, bump envoy when Wolfi has %s\n",
				app, cand, pin, ours, line, pin)
		default:
			fmt.Fprintf(w, "  envoy pairing:      %s %s wants envoy %s; we ship %s -- pairable\n", app, cand, pin, ours)
		}
		return nil
	}
}

// apisix: the release, then our RUNTIME/DASHBOARD_COMMIT pins against .requirements at the newest tag.
func apisixCheck(w io.Writer) error {
	const app = "apisix"
	tags, err := core.Github("apache/apisix")
	if err != nil {
		return err
	}
	behind := report(w, app, tags, 1, filter{})
	var ts []string
	for _, t := range uniq(tags) {
		if core.StartsDigit.MatchString(t) {
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		fmt.Fprintln(w, "  !! could not list upstream tags -- pin audit skipped")
		return nil
	}
	core.SortDesc(ts)
	newest := ts[0]
	out, _ := exec.Command("gh", "api", "repos/apache/apisix/contents/.requirements?ref="+newest,
		"-H", "Accept: application/vnd.github.raw").Output()
	want := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^(APISIX_RUNTIME|APISIX_DASHBOARD_COMMIT)=(\S+)$`).FindAllStringSubmatch(string(out), -1) {
		want[m[1]] = m[2]
	}
	if len(want) == 0 {
		fmt.Fprintf(w, "  !! could not read .requirements at %s -- pin audit skipped\n", newest)
		return nil
	}
	fmt.Fprintf(w, "  .requirements @ %s:\n", newest)
	var drift []string
	for _, p := range [][2]string{{"APISIX_RUNTIME", "RUNTIME"}, {"APISIX_DASHBOARD_COMMIT", "DASHBOARD_COMMIT"}} {
		have := bashVar(app, `"${`+p[1]+`[${VERSIONS[${#VERSIONS[@]}-1]}]}"`)
		up, ok := want[p[0]]
		if !ok {
			up = "?"
		}
		h := have
		if h == "" {
			h = "<unset>"
		}
		mark := "ok"
		if have != up {
			mark = "DRIFT"
			drift = append(drift, p[1]+": "+h+" -> "+up)
		}
		fmt.Fprintf(w, "    %-24s have=%s  upstream=%s  %s\n", p[0], h, up, mark)
	}
	switch {
	case len(drift) > 0:
		rt := want["APISIX_RUNTIME"]
		if rt == "" {
			rt = "?"
		}
		fmt.Fprintf(w, "  => PIN DRIFT %s\n", common.PyList(drift))
		fmt.Fprintf(w, "     A RUNTIME change means re-reading https://raw.githubusercontent.com/api7/apisix-build-tools/apisix-runtime/%s/build-apisix-runtime.sh for the new OpenResty / module / patch versions.\n", rt)
	case len(behind) == 0:
		fmt.Fprintln(w, "  => pins ok")
	}
	return nil
}
