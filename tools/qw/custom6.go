package main

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

func init() {
	for app, c := range map[string]check{
		"linkerd-proxy": {run: linkerdProxy},
		"adc":           {run: adcCheck},
		"contour":       {run: envoyPaired("contour", "projectcontour/contour", contourEnvoyPin)},
		"kgateway":      {run: envoyPaired("kgateway", "kgateway-dev/kgateway", kgatewayEnvoyPin)},
		"apisix":        {run: apisixCheck},
	} {
		if _, dup := table[app]; dup {
			panic("check defined twice: " + app)
		}
		table[app] = c
	}
}

// bashVar echoes a bash expression after sourcing an app's build.conf.
func bashVar(app, expr string) string {
	out, _ := exec.Command("bash", "-c", "source "+imagesRoot+"/apps/"+app+"/build.conf; echo "+expr).Output()
	return strings.TrimSpace(string(out))
}

func sortedAsc(vs []string) []string {
	out := append([]string(nil), vs...)
	sortDesc(out)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// linkerd-proxy is paired with each control-plane edge release through its .proxy-version.
func linkerdProxy(w io.Writer) error {
	cp := sortedAsc(current("linkerd-control-plane"))
	ours := sortedAsc(current("linkerd-proxy"))
	var behind, errs []string
	fmt.Fprintln(w, "linkerd-proxy")
	for i, cpv := range cp {
		want := ""
		vs, err := scrape("https://raw.githubusercontent.com/linkerd/linkerd2/edge-"+cpv+"/.proxy-version", `v?(\d+\.\d+\.\d+)`)
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
	rel, err := github("linkerd/linkerd2-proxy")
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
		sortDesc(tags)
		fmt.Fprintf(w, "  newest upstream proxy release (not yet paired): %s\n", tags[0])
	}
	switch {
	case len(errs) > 0:
		fmt.Fprintf(w, "  => BROKEN CHECK -- could not read .proxy-version: %s\n", pyList(errs))
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
	ctl := current(ctlApp)
	if len(ctl) == 0 {
		return fmt.Errorf("adc: cannot read apps/%s/build.conf VERSIONS -- broken check", ctlApp)
	}
	ctlVer := ctl[len(ctl)-1]
	if paired := bashVar("adc", "${PAIRED_CONTROLLER:-}"); paired != ctlVer {
		return fmt.Errorf("adc: build.conf PAIRED_CONTROLLER=%q but we ship %s %s -- the sidecar/controller pair has drifted", paired, ctlApp, ctlVer)
	}
	url := "https://raw.githubusercontent.com/apache/" + ctlApp + "/" + ctlVer + "/Makefile"
	mk, err := fetch(url)
	if err != nil {
		return fmt.Errorf("adc: cannot fetch %s: %v -- broken check", url, err)
	}
	m := regexp.MustCompile(`(?m)^ADC_VERSION\s*\?=\s*([0-9][^\s#]*)`).FindStringSubmatch(string(mk))
	if m == nil {
		return fmt.Errorf("adc: no ADC_VERSION in %s %s Makefile -- the pin moved", ctlApp, ctlVer)
	}
	pinned := m[1]
	fmt.Fprintf(w, "  pinned by %s %s (Makefile ADC_VERSION): %s\n", ctlApp, ctlVer, pinned)
	tags, err := githubTags("api7/adc")
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
		sortDesc(vs)
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
	b, err := fetch("https://raw.githubusercontent.com/projectcontour/contour/v" + tag + "/examples/contour/03-envoy.yaml")
	if err != nil {
		return "", fmt.Sprintf("could not fetch 03-envoy.yaml (%v)", err)
	}
	if m := regexp.MustCompile(`image:\s*docker\.io/envoyproxy/envoy:(?:distroless-)?v([0-9.]+)`).FindSubmatch(b); m != nil {
		return string(m[1]), ""
	}
	return "", "no envoyproxy/envoy image in 03-envoy.yaml"
}

func kgatewayEnvoyPin(tag string) (string, string) {
	b, err := fetch("https://raw.githubusercontent.com/kgateway-dev/kgateway/v" + tag + "/Makefile")
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
		tags, err := github(repo)
		if err != nil {
			return err
		}
		behind := report(w, app, tags, 0, filter{})
		pool := behind
		if len(pool) == 0 {
			pool = current(app)
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
		for _, v := range current("envoy") {
			lines[strings.Join(strings.SplitN(v, ".", 3)[:2], ".")] = v
		}
		line := strings.Join(strings.SplitN(pin, ".", 3)[:2], ".")
		ours, ok := lines[line]
		switch {
		case !ok:
			fmt.Fprintf(w, "  envoy pairing:      %s %s wants envoy %s; we ship %s -- NO %s LINE, cannot pair (do not bump)\n",
				app, cand, pin, pyList(sortedAsc(keys(lines))), line)
		case vless(ours, pin):
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
	tags, err := github("apache/apisix")
	if err != nil {
		return err
	}
	behind := report(w, app, tags, 1, filter{})
	var ts []string
	for _, t := range uniq(tags) {
		if startsDigit.MatchString(t) {
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		fmt.Fprintln(w, "  !! could not list upstream tags -- pin audit skipped")
		return nil
	}
	sortDesc(ts)
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
		fmt.Fprintf(w, "  => PIN DRIFT %s\n", pyList(drift))
		fmt.Fprintf(w, "     A RUNTIME change means re-reading https://raw.githubusercontent.com/api7/apisix-build-tools/apisix-runtime/%s/build-apisix-runtime.sh for the new OpenResty / module / patch versions.\n", rt)
	case len(behind) == 0:
		fmt.Fprintln(w, "  => pins ok")
	}
	return nil
}
