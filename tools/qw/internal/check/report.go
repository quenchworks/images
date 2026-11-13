package check

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/quenchworks/images/tools/qw/internal/common"
	"github.com/quenchworks/images/tools/qw/internal/core"
)

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
		if core.StartsDigit.MatchString(c) {
			out = append(out, c)
		}
	}
	return out
}

// report compares the newest n upstream versions with what we ship. An empty upstream
// result, or upstream older than us, is a BROKEN CHECK and never "ok" (mongodb sat at
// latest3=[] for weeks while the old checker printed ok).
func report(w io.Writer, app string, cands []string, n int, f filter) []string {
	// Oldest first: bump puts a new version FIRST and the chart-pinned one stays last, so
	// "last entry" read opa as behind on the 1.22.4 it already shipped (2026-11-13).
	cur := sortedAsc(core.Current(app))
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
	core.SortDesc(top)
	if len(top) > n {
		top = top[:n]
	}
	have := []int{0}
	if len(cur) > 0 {
		have = core.Vkey(cur[len(cur)-1])
	}
	var behind []string
	for _, v := range top {
		if core.CmpKey(core.Vkey(v), have) > 0 {
			behind = append(behind, v)
		}
	}
	var status string
	switch {
	case len(top) == 0:
		status = "BROKEN CHECK -- upstream returned NO candidates (not 'up to date')"
	case core.CmpKey(core.Vkey(top[0]), have) < 0:
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
				if strings.HasPrefix(v, p) && strings.Count(v, ".") == strings.Count(c, ".") && !shipped[v] && core.Vless(c, v) {
					if best == "" || core.Vless(best, v) {
						best = v
					}
				}
			}
			if best != "" && !slices.Contains(behind, best) {
				behind = append(behind, best)
			}
		}
		status = "ok"
		if len(behind) > 0 {
			status = "UPDATE -> " + common.PyList(behind)
		}
	}
	h := "?"
	if len(cur) > 0 {
		h = cur[len(cur)-1]
	}
	fmt.Fprintf(w, "%-20s have=%14s  latest%d=%s  %s\n", app, h, n, common.PyList(top), status)
	return behind
}

// reportLines: newest per version line (depth numeric components), against our newest
// entry on each line, plus EVERY newer line (sealed-secrets sat two lines behind).
func reportLines(w io.Writer, app string, cands []string, depth int, f filter) []string {
	cur := core.Current(app)
	wmap := map[string]string{}
	for _, c := range f.apply(cands) {
		lk := core.LineKey(c, depth)
		if old, ok := wmap[lk]; !ok || core.Vless(old, c) {
			wmap[lk] = c
		}
	}
	return lineReport(w, app, cur, wmap, depth, "", false)
}

func lineReport(w io.Writer, app string, cur []string, wmap map[string]string, depth int, label string, lineOnly bool) []string {
	fmt.Fprintln(w, app)
	top := map[string]string{}
	for _, c := range cur {
		lk := core.LineKey(c, depth)
		if old, ok := top[lk]; !ok || core.Vless(old, c) {
			top[lk] = c
		}
	}
	var behind []string
	for _, c := range cur {
		lk := core.LineKey(c, depth)
		if label == "" && top[lk] != c {
			continue // two shipped entries on one line: only the newest is compared
		}
		v, ok := wmap[lk]
		upd := false
		if ok {
			if lineOnly {
				upd = core.CmpKey(core.Vkey(core.LineKey(v, depth)), core.Vkey(c)) > 0
			} else {
				upd = core.Vless(c, v)
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
		if k := core.Vkey(core.LineKey(c, depth)); core.CmpKey(k, oursTop) > 0 {
			oursTop = k
		}
	}
	var newer []string
	for k := range wmap {
		if core.CmpKey(core.Vkey(k), oursTop) > 0 {
			newer = append(newer, k)
		}
	}
	core.SortDesc(newer)
	for _, k := range newer {
		fmt.Fprintf(w, "  NEW LINE available: %s%s (%s)\n", label, k, wmap[k])
	}
	switch {
	case len(behind) > 0:
		fmt.Fprintf(w, "  => UPDATE -> %s\n", common.PyList(behind))
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
	majors, err := core.WolfiMajors(prefix, 12)
	if err != nil {
		return nil, err
	}
	wmap := map[string]string{}
	for _, m := range majors {
		x, _, _ := strings.Cut(m[1], "-r")
		x = gitSuffix.ReplaceAllString(x, "") // 3.14.7_git20260925 is a snapshot of 3.14.7
		wmap[core.LineKey(x, depth)] = x
	}
	return lineReport(w, app, core.Current(app), wmap, depth, prefix, lineOnly), nil
}

// reportWolfiSuffix: apks named <x>-<major>-<y> (dotnet-8-sdk); the first number in the
// name is the line, compared with our entry on that line.
func reportWolfiSuffix(app, re, label string) func(io.Writer) error {
	return func(w io.Writer) error {
		m, err := core.WolfiMatch(re)
		if err != nil {
			return err
		}
		byLine := map[string]string{}
		for p, v := range m {
			maj := core.Digits.FindString(p)
			if old, ok := byLine[maj]; !ok || core.Vless(old, v) {
				byLine[maj] = v
			}
		}
		fmt.Fprintln(w, app)
		var behind []string
		for _, c := range core.Current(app) {
			maj := core.Digits.FindString(c)
			v, ok := byLine[maj]
			upd := ok && core.Vless(c, v)
			if upd {
				behind = append(behind, v)
			}
			if !ok {
				v = "?"
			}
			mark := "ok"
			if upd {
				mark = "UPDATE"
			}
			fmt.Fprintf(w, "  %s: have=%10s  latest=%10s  %s\n", strings.ReplaceAll(label, "{maj}", maj), c, v, mark)
		}
		if len(behind) > 0 {
			fmt.Fprintf(w, "  => UPDATE -> %s\n", common.PyList(behind))
		} else {
			fmt.Fprintln(w, "  => ok")
		}
		return nil
	}
}
