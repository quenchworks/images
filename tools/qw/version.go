package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var digits = regexp.MustCompile(`\d+`)

// vkey is every run of digits in v, so "REL_17_2" and "17.2" order the same way.
func vkey(v string) []int {
	var k []int
	for _, d := range digits.FindAllString(v, -1) {
		n, _ := strconv.Atoi(d)
		k = append(k, n)
	}
	return k
}

// cmpKey compares like Python tuples: element by element, then the shorter is smaller.
func cmpKey(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return len(a) - len(b)
}

func vless(a, b string) bool { return cmpKey(vkey(a), vkey(b)) < 0 }

// lineKey is the first depth numeric components of v, joined with dots.
func lineKey(v string, depth int) string {
	d := digits.FindAllString(v, -1)
	if len(d) > depth {
		d = d[:depth]
	}
	return strings.Join(d, ".")
}

func sortDesc(vs []string) {
	sort.SliceStable(vs, func(i, j int) bool { return vless(vs[j], vs[i]) })
}

var startsDigit = regexp.MustCompile(`^\d`)
