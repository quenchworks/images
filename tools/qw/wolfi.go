package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const wolfiIndex = "https://packages.wolfi.dev/os/%s/APKINDEX.tar.gz"

var (
	wolfiMu  sync.Mutex
	wolfiIdx = map[string]map[string][]string{}
)

// apkKey orders "10.33.0-r1": the X.Y.Z digits, then the -rN revision.
func apkKey(v string) []int {
	xyz, rev, _ := strings.Cut(v, "-r")
	r, _ := strconv.Atoi(regexp.MustCompile(`\D`).ReplaceAllString(rev, ""))
	return append(vkey(xyz), -1, r) // -1 separates, so a longer X.Y.Z still wins
}

// loadWolfi is the x86_64 index, which every check reads.
func loadWolfi() (map[string][]string, error) { return loadWolfiArch("x86_64") }

// loadWolfiArch reads one arch's APKINDEX once per run: package -> every published version.
func loadWolfiArch(arch string) (map[string][]string, error) {
	wolfiMu.Lock()
	defer wolfiMu.Unlock()
	if idx, ok := wolfiIdx[arch]; ok {
		return idx, nil
	}
	idx, err := fetchWolfi(fmt.Sprintf(wolfiIndex, arch))
	if err == nil {
		wolfiIdx[arch] = idx
	}
	return idx, err
}

func fetchWolfi(url string) (map[string][]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("APKINDEX not found in %s: %v", url, err)
		}
		if h.Name == "APKINDEX" {
			break
		}
	}
	out := map[string][]string{}
	var p, v string
	flush := func() {
		if p != "" && v != "" {
			out[p] = append(out[p], v)
		}
		p, v = "", ""
	}
	sc := bufio.NewScanner(tr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		switch ln := sc.Text(); {
		case ln == "":
			flush()
		case strings.HasPrefix(ln, "P:"):
			p = ln[2:]
		case strings.HasPrefix(ln, "V:"):
			v = ln[2:]
		}
	}
	flush()
	return out, sc.Err()
}

// bestPerRelease keeps the highest -rN of each X.Y.Z, newest first.
func bestPerRelease(versions []string) []string {
	by := map[string]string{}
	for _, v := range versions {
		x, _, _ := strings.Cut(v, "-r")
		if cur, ok := by[x]; !ok || cmpKey(apkKey(v), apkKey(cur)) > 0 {
			by[x] = v
		}
	}
	out := make([]string, 0, len(by))
	for _, v := range by {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return cmpKey(apkKey(out[i]), apkKey(out[j])) > 0 })
	return out
}

// wolfiMajors: for prefix "go-", the newest version of each go-<N> package, newest N lines first.
func wolfiMajors(prefix string, n int) ([][2]string, error) {
	idx, err := loadWolfi()
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `(\d+(?:\.\d+)*)$`)
	var names []string
	for p := range idx {
		if re.MatchString(p) {
			names = append(names, p)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return cmpKey(vkey(re.FindStringSubmatch(names[i])[1]), vkey(re.FindStringSubmatch(names[j])[1])) > 0
	})
	if len(names) > n {
		names = names[:n]
	}
	var out [][2]string
	for _, p := range names {
		out = append(out, [2]string{p, bestPerRelease(idx[p])[0]})
	}
	return out, nil
}

// cmdWolfiLatest keeps the wolfi-latest.py command line: <pkg> [N] | --majors <prefix> [N] | --match <re>.
func cmdWolfiLatest(args []string) int {
	idx, err := loadWolfi()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	switch {
	case len(args) > 1 && args[0] == "--majors":
		n := 4
		if len(args) > 2 {
			n, _ = strconv.Atoi(args[2])
		}
		ms, _ := wolfiMajors(args[1], n)
		for _, m := range ms {
			fmt.Fprintf(w, "%s  ->  %s\n", m[0], m[1])
		}
	case len(args) > 1 && args[0] == "--match":
		re := regexp.MustCompile(args[1])
		var names []string
		for p := range idx {
			if re.MatchString(p) {
				names = append(names, p)
			}
		}
		sort.Strings(names)
		for _, p := range names {
			fmt.Fprintf(w, "%s\t%s\n", p, bestPerRelease(idx[p])[0])
		}
	case len(args) > 0:
		n := 3
		if len(args) > 1 {
			n, _ = strconv.Atoi(args[1])
		}
		vs, ok := idx[args[0]]
		if !ok {
			fmt.Fprintf(os.Stderr, "NOT IN WOLFI: %s\n", args[0])
			return 1
		}
		for i, v := range bestPerRelease(vs) {
			if i == n {
				break
			}
			fmt.Fprintln(w, v)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: qw wolfi-latest <pkg> [N] | --majors <prefix> [N] | --match <regex>")
		return 2
	}
	return 0
}
