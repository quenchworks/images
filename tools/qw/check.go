package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// check is one app's upstream version check. Exactly one of src, wolfiPrefix or run is set.
type check struct {
	src         func() ([]string, error)
	lines       int // >0: per-line report keyed to this many numeric components
	n           int // report window; 0 = as many as we ship
	f           filter
	wolfiPrefix string // per-line against Wolfi's prefix-<line> packages
	lineOnly    bool
	run         func(w io.Writer) error // anything the fields above cannot say
}

func ghRel(r string) func() ([]string, error) { return func() ([]string, error) { return github(r) } }
func ghTags(r string) func() ([]string, error) {
	return func() ([]string, error) { return githubTags(r) }
}
func ghAssets(r string) func() ([]string, error) {
	return func() ([]string, error) { return githubAssetReleases(r) }
}
func npmPkg(p string) func() ([]string, error)   { return func() ([]string, error) { return npm(p) } }
func pypiPkg(p string) func() ([]string, error)  { return func() ([]string, error) { return pypi(p) } }
func wolfiPkg(p string) func() ([]string, error) { return func() ([]string, error) { return wolfi(p) } }
func scrapeSrc(u, re string) func() ([]string, error) {
	return func() ([]string, error) { return scrape(u, re) }
}

// full keeps candidates the whole of which match re (Python's re.fullmatch).
func full(re string) func(string) bool {
	r := regexp.MustCompile(`^(?:` + re + `)$`)
	return r.MatchString
}

func (c check) exec(app string, w io.Writer) error {
	switch {
	case c.run != nil:
		return c.run(w)
	case c.wolfiPrefix != "":
		_, err := reportWolfiLines(w, app, c.wolfiPrefix, c.lines, c.lineOnly)
		return err
	}
	cands, err := c.src()
	if err != nil {
		return err
	}
	if c.lines > 0 {
		reportLines(w, app, cands, c.lines, c.f)
	} else {
		report(w, app, cands, c.n, c.f)
	}
	return nil
}

func runOne(app string) string {
	c, ok := table[app]
	if !ok {
		return fmt.Sprintf("%-20s ERROR: no check in tools/qw/table.go", app)
	}
	var b bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- c.exec(app, &b) }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Sprintf("%-20s ERROR: %v", app, err)
		}
		return strings.TrimSpace(b.String())
	case <-time.After(180 * time.Second):
		return fmt.Sprintf("%-20s ERROR: timed out", app)
	}
}

var statusLine = regexp.MustCompile(`(?m)^#\s*STATUS`)

// holdReason: why a newer upstream is not an update (EOL=1, BLOCKED=1, or the STATUS line
// says HELD/DEFERRED/BLOCKED; "DEFERRED to YYYY-MM-DD" stops holding on that date).
func holdReason(app string) string {
	b, err := os.ReadFile(filepath.Join(imagesRoot, "apps", app, "build.conf"))
	if err != nil {
		return ""
	}
	text := string(b)
	status := ""
	for _, l := range strings.Split(text, "\n") {
		if statusLine.MatchString(l) {
			status = strings.TrimSpace(l[1:])
			break
		}
	}
	cut := func(s string, n int) string {
		if len(s) > n {
			return s[:n]
		}
		return s
	}
	if regexp.MustCompile(`(?m)^EOL=1`).MatchString(text) {
		return "EOL: " + cut(status, 90)
	}
	if regexp.MustCompile(`(?m)^BLOCKED=1`).MatchString(text) {
		if m := regexp.MustCompile(`(?m)^BLOCKED_BY=(\S+)`).FindStringSubmatch(text); m != nil {
			return "BLOCKED: " + m[1]
		}
		return "BLOCKED: " + cut(status, 90)
	}
	if regexp.MustCompile(`\b(HELD|DEFERRED|BLOCKED)\b`).MatchString(status) {
		if m := regexp.MustCompile(`DEFERRED to (\d{4}-\d{2}-\d{2})`).FindStringSubmatch(status); m != nil && m[1] <= time.Now().Format("2006-01-02") {
			return ""
		}
		return cut(status, 110)
	}
	return ""
}

// cmdCheck runs the checks (all, or the named apps) in parallel. Exit code is the number
// of apps with an update, so cron and CI can gate on it.
func cmdCheck(args []string) int {
	quiet, workers := false, 8
	var names []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-q" || a == "--updates-only":
			quiet = true
		case (a == "-j" || a == "--jobs") && i+1 < len(args):
			i++
			workers, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", a)
			return 255
		default:
			names = append(names, a)
		}
	}
	var uncovered []string
	if len(names) == 0 {
		names = keys(table)
		// An app with no check is never reported as behind (vikunja sat two releases
		// behind with HIGH CVEs that way), so list them loudly.
		dirs, _ := os.ReadDir(filepath.Join(imagesRoot, "apps"))
		for _, d := range dirs {
			if _, err := os.Stat(filepath.Join(imagesRoot, "apps", d.Name(), "build.conf")); err == nil {
				if _, ok := table[d.Name()]; !ok {
					uncovered = append(uncovered, d.Name())
				}
			}
		}
		sort.Strings(uncovered)
	}
	sort.Strings(names)
	out := make([]string, len(names))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, a := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			out[i] = runOne(a)
			<-sem
		}()
	}
	wg.Wait()

	var updates, current, errs, blank []string
	type held struct{ line, why string }
	var helds []held
	for _, l := range out {
		switch {
		case !strings.Contains(l, "have="):
			errs = append(errs, l)
		case strings.Contains(l, "UPDATE ->"):
			if h := holdReason(strings.Fields(l)[0]); h != "" {
				helds = append(helds, held{l, h})
			} else {
				updates = append(updates, l)
			}
		default:
			current = append(current, l)
		}
		if strings.Contains(l, "have=") && strings.Contains(l, "latest") && strings.Contains(l, "=[]") {
			blank = append(blank, l)
		}
	}
	if quiet {
		if len(updates) == 0 {
			fmt.Println("all current")
		} else {
			fmt.Println(strings.Join(updates, "\n"))
		}
	} else {
		for _, l := range append(current, updates...) {
			fmt.Println(l)
		}
		if len(helds) > 0 {
			fmt.Println("\n-- held (newer upstream, not taken; reason from build.conf) --")
			for _, h := range helds {
				fmt.Printf("%-24s %s\n", strings.Fields(h.line)[0], h.why)
			}
		}
		if len(errs) > 0 {
			fmt.Println("\n-- errors --\n" + strings.Join(errs, "\n"))
		}
	}
	if len(blank) > 0 {
		fmt.Println("\n-- checks returning ZERO candidates (treat as BROKEN, not current) --\n" + strings.Join(blank, "\n"))
	}
	if len(uncovered) > 0 {
		fmt.Printf("\n-- %d apps with NO check (never reported as behind) --\n", len(uncovered))
		for i := 0; i < len(uncovered); i += 8 {
			fmt.Println("   " + strings.Join(uncovered[i:min(i+8, len(uncovered))], " "))
		}
	}
	fmt.Printf("\nsummary: %d/%d apps checked · %d updates · %d held · %d errors · %d blank · %d unchecked\n",
		len(names), len(names)+len(uncovered), len(updates), len(helds), len(errs), len(blank), len(uncovered))
	return len(updates)
}
