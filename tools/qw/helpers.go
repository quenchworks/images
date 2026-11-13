package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

func prefix(p string) func(string) bool {
	return func(v string) bool { return strings.HasPrefix(v, p) }
}
func notPrefix(p string) func(string) bool {
	return func(v string) bool { return !strings.HasPrefix(v, p) }
}
func noDash(v string) bool { return !strings.Contains(v, "-") }

// numDots is Python's v.replace(".", "").isdigit(): only digits and dots, at least one digit.
func numDots(v string) bool {
	s := strings.ReplaceAll(v, ".", "")
	return s != "" && strings.Trim(s, "0123456789") == ""
}

func trim(p string) func(string) string {
	return func(v string) string { return strings.TrimPrefix(v, p) }
}

// group keeps candidates matching re in full and maps each to its first group.
func group(re string) filter {
	r := regexp.MustCompile(`^(?:` + re + `)$`)
	return filter{keep: r.MatchString, clean: func(v string) string { return r.FindStringSubmatch(v)[1] }}
}

// mapThen cleans first and keeps on the cleaned value (Python's "re.sub, then fullmatch").
func mapThen(clean func(string) string, keep func(string) bool) filter {
	return filter{keep: func(v string) bool { return keep(clean(v)) }, clean: clean}
}

// wolfiVals: the newest version of every Wolfi apk whose name matches re.
func wolfiVals(re string) func() ([]string, error) {
	return func() ([]string, error) {
		m, err := wolfiMatch(re)
		return valsOf(m), err
	}
}

func valsOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// jsonList fetches url and returns pick(decoded).
func jsonList(url string, pick func(any) []string) func() ([]string, error) {
	return func() ([]string, error) {
		var d any
		if err := jsonGet(url, &d); err != nil {
			return nil, err
		}
		return pick(d), nil
	}
}

// path walks decoded JSON: strings index objects, ints index arrays.
func path(d any, keys ...any) any {
	for _, k := range keys {
		switch k := k.(type) {
		case string:
			m, _ := d.(map[string]any)
			d = m[k]
		case int:
			a, _ := d.([]any)
			if k >= len(a) {
				return nil
			}
			d = a[k]
		}
	}
	return d
}

func str(d any) string { s, _ := d.(string); return s }

// each maps every element of a JSON array through f, dropping empty results.
func each(d any, f func(any) string) []string {
	a, _ := d.([]any)
	var out []string
	for _, e := range a {
		if s := f(e); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// reportWolfiSuffix: apks named <x>-<major>-<y> (dotnet-8-sdk); the first number in the
// name is the line, compared with our entry on that line.
func reportWolfiSuffix(app, re, label string) func(io.Writer) error {
	return func(w io.Writer) error {
		m, err := wolfiMatch(re)
		if err != nil {
			return err
		}
		byLine := map[string]string{}
		for p, v := range m {
			maj := digits.FindString(p)
			if old, ok := byLine[maj]; !ok || vless(old, v) {
				byLine[maj] = v
			}
		}
		fmt.Fprintln(w, app)
		var behind []string
		for _, c := range current(app) {
			maj := digits.FindString(c)
			v, ok := byLine[maj]
			upd := ok && vless(c, v)
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
			fmt.Fprintf(w, "  => UPDATE -> %s\n", pyList(behind))
		} else {
			fmt.Fprintln(w, "  => ok")
		}
		return nil
	}
}
