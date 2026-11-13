package main

// qw unbuilt finds recipes whose newest VERSIONS entry was never published. Build
// workflows are dispatch-only, so a committed bump can sit ahead of the registry with
// every run green (zot and three linkerd apps, found 2026-09-17). BLOCKED apps are
// expected to be behind and are skipped unless --all.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func published(app, ver string) bool {
	out, err := exec.Command("docker", "buildx", "imagetools", "inspect",
		"ghcr.io/quenchworks/images/"+app+":"+ver, "--format", "{{.Manifest.Digest}}").Output()
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "sha256:")
}

func cmdUnbuilt(args []string) int {
	all := false
	var apps []string
	for _, a := range args {
		if a == "--all" {
			all = true
		} else {
			apps = append(apps, a)
		}
	}
	if len(apps) == 0 {
		dirs, _ := os.ReadDir(filepath.Join(imagesRoot, "apps"))
		for _, d := range dirs {
			if _, err := os.Stat(filepath.Join(imagesRoot, "apps", d.Name(), "build.conf")); err == nil {
				apps = append(apps, d.Name())
			}
		}
		sort.Strings(apps)
	}
	blockedRe := regexp.MustCompile(`(?m)^BLOCKED=1`)
	var real []string
	skipped := 0
	for _, app := range apps {
		b, err := os.ReadFile(filepath.Join(imagesRoot, "apps", app, "build.conf"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "!! no such app: %s\n", app)
			continue
		}
		blocked := blockedRe.Match(b)
		if blocked && !all {
			skipped++
			continue
		}
		vs := current(app)
		if len(vs) == 0 {
			fmt.Fprintf(os.Stderr, "!! %s: could not read VERSIONS\n", app)
			continue
		}
		// newest by version: bump puts a new entry first, the chart-pinned one stays last
		sortDesc(vs)
		if !published(app, vs[0]) {
			note := ""
			if blocked {
				note = "  (BLOCKED, expected)"
			} else {
				real = append(real, app)
			}
			fmt.Printf("UNBUILT %s %s%s\n", app, vs[0], note)
		}
	}
	fmt.Printf("\nchecked %d apps, skipped %d blocked, %d recipe(s) ahead of the registry\n", len(apps)-skipped, skipped, len(real))
	if len(real) > 0 {
		var cmds []string
		for _, a := range real {
			cmds = append(cmds, "gh workflow run build-"+a+".yml -R quenchworks/images")
		}
		fmt.Println("dispatch them: " + strings.Join(cmds, " "))
		return 1
	}
	return 0
}
