package main

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// imagesRoot is the images/ checkout; main sets it.
var imagesRoot = "."

// current is the app's VERSIONS array from its build.conf (bash, because build.conf is bash).
func current(app string) []string {
	out, _ := exec.Command("bash", "-c", "source "+imagesRoot+"/apps/"+app+"/build.conf; echo ${VERSIONS[@]}").Output()
	return strings.Fields(string(out))
}

// pyList prints like Python's repr of a list of strings, so the output matches the old checkers.
func pyList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = "'" + v + "'"
	}
	return "[" + strings.Join(q, ", ") + "]"
}

type filter struct {
	keep  func(string) bool
	clean func(string) string
}

func (f filter) apply(cands []string) []string {
	var out []string
	for _, c := range cands {
		if f.keep != nil && !f.keep(c) {
			continue
		}
		if f.clean != nil {
			c = f.clean(c)
		}
		if startsDigit.MatchString(c) {
			out = append(out, c)
		}
	}
	return out
}

// report compares the newest n upstream versions with what we ship. An empty upstream
// result, or upstream older than us, is a BROKEN CHECK and never "ok" (mongodb sat at
// latest3=[] for weeks while the old checker printed ok).
func report(w io.Writer, app string, cands []string, n int, f filter) []string {
	cur := current(app)
	if n == 0 {
		n = len(cur)
		if n == 0 {
			n = 3
		}
	}
	all := f.apply(cands)
	seen := map[string]bool{}
	var top []string
	for _, c := range all {
		if !seen[c] {
			seen[c] = true
			top = append(top, c)
		}
	}
	sortDesc(top)
	if len(top) > n {
		top = top[:n]
	}
	have := []int{0}
	if len(cur) > 0 {
		have = vkey(cur[len(cur)-1])
	}
	var behind []string
	for _, v := range top {
		if cmpKey(vkey(v), have) > 0 {
			behind = append(behind, v)
		}
	}
	var status string
	switch {
	case len(top) == 0:
		status = "BROKEN CHECK -- upstream returned NO candidates (not 'up to date')"
	case cmpKey(vkey(top[0]), have) < 0:
		status = fmt.Sprintf("BROKEN CHECK -- we ship %s but upstream's newest is %s; wrong source?", cur[len(cur)-1], top[0])
	default:
		// A patch on an OLDER line we still ship is outside the newest-n window
		// (cert-manager 1.20.3 while 1.20.4 existed): flag it per line.
		shipped := map[string]bool{}
		for _, c := range cur {
			shipped[c] = true
		}
		pre := func(v string) string { return v[:strings.LastIndex(v, ".")+1] }
		newestPre := pre(cur[len(cur)-1])
		for _, c := range cur[:len(cur)-1] {
			p := pre(c)
			if p == newestPre {
				continue
			}
			best := ""
			for _, v := range all {
				if strings.HasPrefix(v, p) && strings.Count(v, ".") == strings.Count(c, ".") && !shipped[v] && vless(c, v) {
					if best == "" || vless(best, v) {
						best = v
					}
				}
			}
			if best != "" && !contains(behind, best) {
				behind = append(behind, best)
			}
		}
		status = "ok"
		if len(behind) > 0 {
			status = "UPDATE -> " + pyList(behind)
		}
	}
	h := "?"
	if len(cur) > 0 {
		h = cur[len(cur)-1]
	}
	fmt.Fprintf(w, "%-20s have=%14s  latest%d=%s  %s\n", app, h, n, pyList(top), status)
	return behind
}

func contains(vs []string, s string) bool {
	for _, v := range vs {
		if v == s {
			return true
		}
	}
	return false
}

// reportLines: newest per version line (depth numeric components), against our newest
// entry on each line, plus EVERY newer line (sealed-secrets sat two lines behind).
func reportLines(w io.Writer, app string, cands []string, depth int, f filter) []string {
	cur := current(app)
	wmap := map[string]string{}
	for _, c := range f.apply(cands) {
		lk := lineKey(c, depth)
		if old, ok := wmap[lk]; !ok || vless(old, c) {
			wmap[lk] = c
		}
	}
	return lineReport(w, app, cur, wmap, depth, "", false)
}

func lineReport(w io.Writer, app string, cur []string, wmap map[string]string, depth int, label string, lineOnly bool) []string {
	fmt.Fprintln(w, app)
	top := map[string]string{}
	for _, c := range cur {
		lk := lineKey(c, depth)
		if old, ok := top[lk]; !ok || vless(old, c) {
			top[lk] = c
		}
	}
	var behind []string
	for _, c := range cur {
		lk := lineKey(c, depth)
		if label == "" && top[lk] != c {
			continue // two shipped entries on one line: only the newest is compared
		}
		v, ok := wmap[lk]
		upd := false
		if ok {
			if lineOnly {
				upd = cmpKey(vkey(lineKey(v, depth)), vkey(c)) > 0
			} else {
				upd = vless(c, v)
			}
		}
		if upd {
			behind = append(behind, v)
		}
		shown := v
		if !ok {
			shown = "?"
		}
		mark := "ok"
		if upd {
			mark = "UPDATE"
		}
		fmt.Fprintf(w, "  %s%s: have=%12s  latest=%12s  %s\n", label, lk, c, shown, mark)
	}
	oursTop := []int{0}
	for _, c := range cur {
		if k := vkey(lineKey(c, depth)); cmpKey(k, oursTop) > 0 {
			oursTop = k
		}
	}
	var newer []string
	for k := range wmap {
		if cmpKey(vkey(k), oursTop) > 0 {
			newer = append(newer, k)
		}
	}
	sortDesc(newer)
	for _, k := range newer {
		fmt.Fprintf(w, "  NEW LINE available: %s%s (%s)\n", label, k, wmap[k])
	}
	switch {
	case len(behind) > 0:
		fmt.Fprintf(w, "  => UPDATE -> %s\n", pyList(behind))
	case len(newer) > 0:
		fmt.Fprintf(w, "  => NEW LINE %s\n", strings.Join(newer, " "))
	default:
		fmt.Fprintln(w, "  => ok")
	}
	return behind
}

var gitSuffix = regexp.MustCompile(`_git\d+$`)

// reportWolfiLines: per shipped line, Wolfi's newest patch of prefix+<line> (go-1.26,
// openjdk-21, php-8.4). lineOnly: we pin the line and the patch floats.
func reportWolfiLines(w io.Writer, app, prefix string, depth int, lineOnly bool) ([]string, error) {
	majors, err := wolfiMajors(prefix, 12)
	if err != nil {
		return nil, err
	}
	wmap := map[string]string{}
	for _, m := range majors {
		x, _, _ := strings.Cut(m[1], "-r")
		x = gitSuffix.ReplaceAllString(x, "") // 3.14.7_git20260925 is a snapshot of 3.14.7
		wmap[lineKey(x, depth)] = x
	}
	return lineReport(w, app, current(app), wmap, depth, prefix, lineOnly), nil
}
