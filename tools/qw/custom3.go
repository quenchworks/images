package main

import (
	"strings"
)

func init() {
	for app, c := range map[string]check{
		"apisix-ingress-controller": {src: scrapeSrc("https://archive.apache.org/dist/apisix/ingress-controller/", `href="(\d+\.\d+\.\d+)/"`), n: 1},
		// the newest four 1.x lines, each listing page scraped for its tarballs
		"mediawiki": {src: func() ([]string, error) {
			const base = "https://releases.wikimedia.org/mediawiki/"
			ls, err := scrape(base, `href="(1\.\d+)/"`)
			if err != nil {
				return nil, err
			}
			ls = uniq(ls)
			sortDesc(ls)
			ls = ls[:min(4, len(ls))]
			var out []string
			for _, l := range ls {
				vs, err := scrape(base+l+"/", `mediawiki-(\d+\.\d+\.\d+)\.tar\.gz"`)
				if err != nil {
					return nil, err
				}
				out = append(out, vs...)
			}
			return out, nil
		}, lines: 2},
		"n8n":         {src: n8nStable, lines: 2},
		"n8n-runners": {src: n8nStable, lines: 2},
		"vault":       {src: ghRel("hashicorp/vault"), lines: 2, f: filter{keep: numDots}},
		"yarn": {src: ghRel("yarnpkg/berry"), f: filter{keep: prefix("@yarnpkg/cli/"),
			clean: func(t string) string { return t[strings.LastIndex(t, "/")+1:] }}},
		"zabbix-server": {src: zabbixSources, lines: 2},
		"zabbix-web":    {src: zabbixSources, lines: 2},
		"graylog":       {src: ghTags("Graylog2/graylog2-server"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		// coolify pins its realtime image in versions.json; read it at the newest 4.0-4.3 tag that has one.
		"coolify-realtime": {src: func() ([]string, error) {
			tags, err := github("coollabsio/coolify")
			if err != nil {
				return nil, err
			}
			for _, t := range tags {
				p := strings.SplitN(t, ".", 3)
				// Python compared the first two fields as STRINGS: ["4","10"] < ["4","4"]. Kept as is.
				if len(p) < 2 || !(p[0] < "4" || (p[0] == "4" && p[1] < "4")) {
					continue
				}
				var vj map[string]any
				if jsonGet("https://raw.githubusercontent.com/coollabsio/coolify/v"+t+"/versions.json", &vj) != nil {
					continue
				}
				for _, o := range vj {
					if v := str(path(o, "realtime", "version")); v != "" {
						return []string{v}, nil
					}
				}
			}
			return nil, nil
		}, n: 1},
	} {
		if _, dup := table[app]; dup {
			panic("check defined twice: " + app)
		}
		table[app] = c
	}
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
	if err := jsonGet("https://registry.npmjs.org/n8n", &d); err != nil {
		return nil, err
	}
	line := vkey(lineKey(d.DistTags["stable"], 2))
	var out []string
	for v := range d.Versions {
		if !strings.Contains(v, "-") && cmpKey(vkey(lineKey(v, 2)), line) <= 0 {
			out = append(out, v)
		}
	}
	return out, nil
}

// zabbixSources: every X.Y.Z tarball on the 6.0+ lines of the stable sources tree.
func zabbixSources() ([]string, error) {
	const base = "https://cdn.zabbix.com/zabbix/sources/stable/"
	ls, err := scrape(base, `href="(\d+\.\d+)/"`)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range ls {
		if vkey(l)[0] < 6 {
			continue
		}
		vs, err := scrape(base+l+"/", `zabbix-(\d+\.\d+\.\d+)\.tar\.gz`)
		if err != nil {
			return nil, err
		}
		out = append(out, vs...)
	}
	return out, nil
}
