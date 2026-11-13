package core

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/quenchworks/images/tools/qw/internal/constants"
)

var (
	wolfiMu  sync.Mutex
	wolfiIdx = map[string]map[string][]string{}
)

// apkKey orders "10.33.0-r1": the X.Y.Z digits, then the -rN revision.
func apkKey(v string) []int {
	xyz, rev, _ := strings.Cut(v, "-r")
	r, _ := strconv.Atoi(regexp.MustCompile(`\D`).ReplaceAllString(rev, ""))
	return append(Vkey(xyz), -1, r) // -1 separates, so a longer X.Y.Z still wins
}

// loadWolfi is the x86_64 index, which every check reads.
func LoadWolfi() (map[string][]string, error) { return LoadWolfiArch("x86_64") }

// loadWolfiArch reads one arch's APKINDEX once per run: package -> every published version.
func LoadWolfiArch(arch string) (map[string][]string, error) {
	wolfiMu.Lock()
	defer wolfiMu.Unlock()
	if idx, ok := wolfiIdx[arch]; ok {
		return idx, nil
	}
	idx, err := fetchWolfi(fmt.Sprintf(constants.WolfiIndex, arch))
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
func BestPerRelease(versions []string) []string {
	by := map[string]string{}
	for _, v := range versions {
		x, _, _ := strings.Cut(v, "-r")
		if cur, ok := by[x]; !ok || CmpKey(apkKey(v), apkKey(cur)) > 0 {
			by[x] = v
		}
	}
	out := make([]string, 0, len(by))
	for _, v := range by {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return CmpKey(apkKey(out[i]), apkKey(out[j])) > 0 })
	return out
}

// wolfiMajors: for prefix "go-", the newest version of each go-<N> package, newest N lines first.
func WolfiMajors(prefix string, n int) ([][2]string, error) {
	idx, err := LoadWolfi()
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
		return CmpKey(Vkey(re.FindStringSubmatch(names[i])[1]), Vkey(re.FindStringSubmatch(names[j])[1])) > 0
	})
	if len(names) > n {
		names = names[:n]
	}
	var out [][2]string
	for _, p := range names {
		out = append(out, [2]string{p, BestPerRelease(idx[p])[0]})
	}
	return out, nil
}
