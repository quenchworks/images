package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	for app, c := range map[string]check{
		"proxysql": {run: func(w io.Writer) error {
			vs, err := wolfi("proxysql")
			if err != nil {
				return err
			}
			reportLines(w, "proxysql", vs, 1, filter{})
			up, err := github("sysown/proxysql")
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "  upstream (github.com/sysown/proxysql): %s\n", strings.Join(up[:min(3, len(up))], ", "))
			return nil
		}},
		"bifrost": {src: ghTags("maximhq/bifrost"), lines: 2, f: filter{
			keep:  func(t string) bool { return strings.HasPrefix(t, "transports/v") && !strings.Contains(t, "-") },
			clean: func(t string) string { return strings.TrimLeft(strings.SplitN(t, "/", 2)[1], "v") }}},
		"cassandra": {src: ghTags("apache/cassandra"), f: filter{keep: full(`cassandra-\d+\.\d+\.\d+`), clean: trim("cassandra-")}},
		"scylladb":  {src: ghTags("scylladb/scylladb"), f: filter{keep: full(`scylla-\d+\.\d+\.\d+`), clean: trim("scylla-")}},
		"zookeeper": {src: ghTags("apache/zookeeper"), f: filter{keep: full(`release-\d+\.\d+\.\d+`), clean: trim("release-")}},
		"cluster-autoscaler": {src: ghRel("kubernetes/autoscaler"), lines: 2, f: filter{
			keep: regexp.MustCompile(`^cluster-autoscaler-\d`).MatchString, clean: trim("cluster-autoscaler-")}},
		// deno is frozen at 2.9.7 until its hold clears (apps/deno/build.conf).
		"deno": {src: ghRel("denoland/deno"), f: filter{keep: func(t string) bool {
			for _, p := range strings.Split(t, ".") {
				if p == "" || strings.Trim(p, "0123456789") != "" {
					return false
				}
			}
			return !vless("2.9.7", t)
		}}},
		// RE2 has no lookahead: "not followed by a dot" (skips the .asc/.sha files) as a class.
		"httpd": {src: scrapeSrc("https://downloads.apache.org/httpd/", `httpd-(2\.4\.\d+)\.tar\.bz2(?:[^.]|$)`), n: 1},
		"mongodb": {src: jsonList("https://downloads.mongodb.org/current.json", func(d any) []string {
			return each(path(d, "versions"), func(v any) string { return str(path(v, "version")) })
		}), f: filter{keep: full(`\d+\.0\.\d+`)}},
		"powerdns":          pdnsRefs("auth-"),
		"powerdns-recursor": pdnsRefs("rec-"),
		"storm":             {src: scrapeSrc("https://archive.apache.org/dist/storm/", `href="apache-storm-(\d+\.\d+\.\d+)/"`), n: 1},
		"tomcat":            {src: scrapeSrc("https://downloads.apache.org/tomcat/tomcat-11/", `href="v(11\.0\.\d+)/"`), n: 1},
		"tomee":             {src: scrapeSrc("https://archive.apache.org/dist/tomee/", `href="tomee-(\d+\.\d+\.\d+)/"`), n: 1},
		// github() strips the leading "v" of every tag, so these tags read "ertical-pod-autoscaler-".
		"vertical-pod-autoscaler": {src: ghRel("kubernetes/autoscaler"), lines: 2, f: filter{
			keep: func(t string) bool {
				return strings.HasPrefix(t, "ertical-pod-autoscaler-") && !strings.HasPrefix(t, "ertical-pod-autoscaler-chart")
			},
			clean: trim("ertical-pod-autoscaler-")}},
		"static": {run: func(w io.Writer) error {
			ver := "datestamped"
			if b, err := os.ReadFile(filepath.Join(imagesRoot, "apps", "static", "build.conf")); err == nil {
				if m := regexp.MustCompile(`VERSIONS=\(([^)]*)\)`).FindStringSubmatch(string(b)); m != nil {
					if f := strings.Fields(m[1]); len(f) > 0 {
						ver = f[len(f)-1]
					}
				}
			}
			fmt.Fprintf(w, "static               have=%13s  latest=%13s  ok  (datestamped base, no upstream to track)\n", ver, ver)
			return nil
		}},
		"drupal": {src: scrapeSrc("https://updates.drupal.org/release-history/drupal/current", `<version>(\d+\.\d+\.\d+)</version>`), lines: 2},
		"nginx": {src: ghTags("nginx/nginx"), f: filter{keep: func(t string) bool {
			m := regexp.MustCompile(`^release-\d+\.(\d+)\.\d+$`).FindStringSubmatch(t)
			return m != nil && vkey(m[1])[0]%2 == 0 // even minor = stable
		}, clean: trim("release-")}},
	} {
		if _, dup := table[app]; dup {
			panic("check defined twice: " + app)
		}
		table[app] = c
	}
}

// pdnsRefs: PowerDNS tags auth-X.Y.Z / rec-X.Y.Z, read through the matching-refs API.
func pdnsRefs(p string) check {
	return check{src: func() ([]string, error) {
		return gh("repos/PowerDNS/pdns/git/matching-refs/tags/"+p, ".[].ref")
	}, lines: 2, f: group(`refs/tags/` + p + `(\d+\.\d+\.\d+)`)}
}
