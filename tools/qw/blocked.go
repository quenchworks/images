package main

// qw blocked lists BLOCKED=1 apps and whether the package that walls them has shipped
// the fixed version on BOTH arches; --dispatch then flips BLOCKED=0 on the recipes whose
// build.conf names that package, commits, pushes and rebuilds them.
// A recorded fixed version is a snapshot of an advisory, not a constant: the zlib block
// watched 1.3.3-r0 for months while Wolfi shipped the fix as 1.3.2.1_rc20260601-r0.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// apkVKey orders a Wolfi version: dot/underscore fields (non-numeric = 0), then -rN.
func apkVKey(v string) []int {
	base, rel := v, 0
	if m := regexp.MustCompile(`^(.*?)-r(\d+)$`).FindStringSubmatch(v); m != nil {
		base = m[1]
		rel, _ = strconv.Atoi(m[2])
	}
	var k []int
	for _, p := range regexp.MustCompile(`[._]`).Split(base, -1) {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		k = append(k, n)
	}
	return append(k, -1, rel) // -1 ends the base, so a longer base still wins
}

func wolfiNewest(pkg, arch string) (string, error) {
	idx, err := loadWolfiArch(arch)
	if err != nil {
		return "", err
	}
	best := ""
	for _, v := range idx[pkg] {
		if best == "" || cmpKey(apkVKey(v), apkVKey(best)) > 0 {
			best = v
		}
	}
	return best, nil
}

func cmdBlocked(args []string) int {
	pkg, minV, dispatch := "zlib", "1.3.2.1_rc20260601-r0", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pkg":
			i++
			pkg = args[i]
		case "--min":
			i++
			minV = args[i]
		case "--dispatch":
			dispatch = true
		}
	}
	confs, _ := filepath.Glob(filepath.Join(imagesRoot, "apps", "*", "build.conf"))
	sort.Slice(confs, func(i, j int) bool { return filepath.Dir(confs[i]) < filepath.Dir(confs[j]) })
	blockedRe := regexp.MustCompile(`(?m)^BLOCKED=1`)
	var mine, other []string
	for _, c := range confs {
		b, _ := os.ReadFile(c)
		if !blockedRe.Match(b) {
			continue
		}
		// the CURRENT reason only: the newest STATUS line or BLOCKED_BY. Matching anywhere
		// in the file counted envoy, jellyfin, ml-metadata-store-server and quickwit as
		// zlib-blocked from their history notes, and --dispatch would have unblocked them
		// (2026-11-13; their real walls are Wolfi, SkiaSharp, static C++ libs and thrift).
		if strings.Contains(currentReason(string(b)), pkg) {
			mine = append(mine, c)
		} else {
			other = append(other, filepath.Base(filepath.Dir(c)))
		}
	}
	if len(mine)+len(other) == 0 {
		fmt.Println("nothing blocked")
		return 0
	}
	lifted := true
	var have []string
	for _, arch := range []string{"x86_64", "aarch64"} {
		v, err := wolfiNewest(pkg, arch)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if v == "" || cmpKey(apkVKey(v), apkVKey(minV)) < 0 {
			lifted = false
		}
		if v == "" {
			v = "None"
		}
		have = append(have, arch+"="+v)
	}
	state := "still walled"
	if lifted {
		state = "LIFTED"
	}
	fmt.Printf("%s: %s\nneeds %s -> %s\n\n", pkg, strings.Join(have, ", "), minV, state)
	names := make([]string, len(mine))
	for i, c := range mine {
		names[i] = filepath.Base(filepath.Dir(c))
	}
	fmt.Printf("blocked on %s (%d): %s\n", pkg, len(mine), strings.Join(names, " "))
	if len(other) > 0 {
		fmt.Printf("blocked for other reasons (%d): %s\n", len(other), strings.Join(other, " "))
	}
	if !dispatch || len(mine) == 0 {
		return 0
	}
	if !lifted {
		fmt.Printf("\nrefusing to dispatch: %s has not reached %s\n", pkg, minV)
		return 1
	}
	for _, c := range mine {
		b, _ := os.ReadFile(c)
		loc := blockedRe.FindIndex(b)
		b = append(append(append([]byte{}, b[:loc[0]]...), "BLOCKED=0"...), b[loc[1]:]...)
		if err := os.WriteFile(c, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	x86 := strings.TrimPrefix(have[0], "x86_64=")
	// push BEFORE dispatching: a dispatch runs on the remote main, which without the push
	// still says BLOCKED=1 and skips the build
	for _, cmd := range [][]string{
		append([]string{"git", "add"}, mine...),
		{"git", "commit", "-m", fmt.Sprintf("chore: unblock %d apps, Wolfi shipped %s %s", len(mine), pkg, x86)},
		{"git", "push", "origin", "HEAD:main"},
	} {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir, c.Stdout, c.Stderr = imagesRoot, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return 2
		}
	}
	for _, n := range names {
		c := exec.Command("gh", "workflow", "run", "build-"+n+".yml", "-R", "quenchworks/images")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return 2
		}
	}
	fmt.Printf("dispatched %d; check each run's headSha matches the push\n", len(names))
	return 0
}

// currentReason is the first "# STATUS" line plus BLOCKED_BY, where a block names its cause.
func currentReason(conf string) string {
	var out []string
	if m := regexp.MustCompile(`(?m)^#\s*STATUS.*$`).FindString(conf); m != "" {
		out = append(out, m)
	}
	if m := regexp.MustCompile(`(?m)^BLOCKED_BY=.*$`).FindString(conf); m != "" {
		out = append(out, m)
	}
	return strings.Join(out, "\n")
}
