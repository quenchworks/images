package drift

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/quenchworks/images/tools/qw/internal/common"
	"github.com/quenchworks/images/tools/qw/internal/core"
	"gopkg.in/yaml.v3"
)

type finding struct {
	Pkg       string `json:"pkg"`
	ID        string `json:"id"`
	Severity  string `json:"severity"`
	Installed string `json:"installed"`
	Fixed     string `json:"fixed"`
}

type dirtyImage struct {
	App      string    `json:"app"`
	Version  string    `json:"version"`
	Findings []finding `json:"findings"`
}

// driftScan returns the findings for one image, or nil, false when the scan itself failed.
func driftScan(app, ref string) ([]finding, bool) {
	args := []string{"image", "--ignore-unfixed", "--severity", "CRITICAL,HIGH,MEDIUM,LOW",
		"--scanners", "vuln", "--detection-priority", "comprehensive", "--quiet", "-f", "json"}
	if vex := filepath.Join(core.ImagesRoot, "apps", app, "vex.openvex.json"); fileExists(vex) {
		args = append(args, "--vex", vex)
	}
	out, err := exec.Command("trivy", append(args, ref)...).Output()
	if ee, ok := err.(*exec.ExitError); err != nil && !(ok && ee.ExitCode() == 1) || strings.TrimSpace(string(out)) == "" {
		return nil, false
	}
	var rep struct {
		Results []struct {
			Vulnerabilities []struct {
				PkgName, VulnerabilityID, Severity, InstalledVersion, FixedVersion string
			}
		}
	}
	if json.Unmarshal(out, &rep) != nil {
		return nil, false
	}
	fs := []finding{}
	for _, r := range rep.Results {
		for _, v := range r.Vulnerabilities {
			fs = append(fs, finding{v.PkgName, v.VulnerabilityID, v.Severity, v.InstalledVersion, v.FixedVersion})
		}
	}
	return fs, true
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func Run(args []string) int {
	shard, of, jsonOut := 0, 1, ""
	var only []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--shard":
			i++
			shard, _ = strconv.Atoi(args[i])
		case "--of":
			i++
			of, _ = strconv.Atoi(args[i])
		case "--json":
			i++
			jsonOut = args[i]
		case "--only":
			only = append(only, args[i+1:]...)
			i = len(args)
		}
	}
	b, err := os.ReadFile(filepath.Join(core.ImagesRoot, "catalog.lock.yaml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var lock struct {
		Apps map[string]struct {
			Image    string `yaml:"image"`
			Versions []struct {
				Version   string `yaml:"version"`
				Published string `yaml:"published"`
			} `yaml:"versions"`
		} `yaml:"apps"`
	}
	if err := yaml.Unmarshal(b, &lock); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	type target struct{ app, ver, image string }
	var targets []target
	names := common.Keys(lock.Apps)
	sort.Strings(names)
	for _, n := range names {
		a := lock.Apps[n]
		vs := a.Versions
		// newest first: a shard that stalls has still covered the tags most people pull
		sort.SliceStable(vs, func(i, j int) bool { return vs[i].Published > vs[j].Published })
		for _, v := range vs {
			targets = append(targets, target{n, v.Version, a.Image})
		}
	}
	var sel []target
	want := map[string]bool{}
	for _, a := range only {
		want[a] = true
	}
	for i, t := range targets {
		if (len(only) > 0 && want[t.app]) || (len(only) == 0 && i%of == shard) {
			sel = append(sel, t)
		}
	}
	dirty, failed := []dirtyImage{}, []string{}
	for _, t := range sel {
		fs, ok := driftScan(t.app, t.image+":"+t.ver)
		switch {
		case !ok:
			failed = append(failed, t.app+" "+t.ver)
			fmt.Printf("%-28s %-14s SCAN FAILED\n", t.app, t.ver)
		case len(fs) > 0:
			dirty = append(dirty, dirtyImage{t.app, t.ver, fs})
			sev := map[string]bool{}
			for _, f := range fs {
				s := f.Severity
				if s == "" {
					s = "?"
				}
				sev[s] = true
			}
			fmt.Printf("%-28s %-14s %d finding(s) [%s]\n", t.app, t.ver, len(fs), strings.Join(slices.Sorted(maps.Keys(sev)), ","))
		default:
			fmt.Printf("%-28s %-14s clean\n", t.app, t.ver)
		}
	}
	fmt.Printf("\nscanned %d | dirty %d | scan-failed %d\n", len(sel), len(dirty), len(failed))
	if jsonOut != "" {
		b, _ := json.MarshalIndent(map[string]any{"dirty": dirty, "failed": failed}, "", " ")
		if err := os.WriteFile(jsonOut, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if len(dirty)+len(failed) > 0 {
		return 1
	}
	return 0
}
