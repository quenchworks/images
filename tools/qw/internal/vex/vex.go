package vex

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/quenchworks/images/tools/qw/internal/common"
	"github.com/quenchworks/images/tools/qw/internal/core"
	"gopkg.in/yaml.v3"
)

var justifications = map[string]bool{
	"component_not_present":                             true,
	"vulnerable_code_not_present":                       true,
	"vulnerable_code_not_in_execute_path":               true,
	"vulnerable_code_cannot_be_controlled_by_adversary": true,
	"inline_mitigations_already_exist":                  true,
}

const minImpact = 120

// Words that show the author looked rather than assumed; broad on purpose.
var evidenceMarkers = []string{"verified", "verify", "confirmed", "extract", "grep", "0 matches", "no matches",
	"inspected", "checked", "reproduce", "does not ship", "not present", "absent",
	"measured", "read the source", "disassembl", "strings ", "sbom"}

type vexDoc struct {
	Context    any    `json:"@context"`
	ID         string `json:"@id"`
	Author     string `json:"author"`
	Timestamp  string `json:"timestamp"`
	Statements []struct {
		Vulnerability struct {
			Name string `json:"name"`
		} `json:"vulnerability"`
		Products []struct {
			ID string `json:"@id"`
		} `json:"products"`
		Status          any    `json:"status"`
		Justification   any    `json:"justification"`
		ImpactStatement string `json:"impact_statement"`
	} `json:"statements"`
}

func checkVexFile(path string) ([]string, *vexDoc) {
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}, nil
	}
	var doc vexDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return []string{"invalid JSON: " + err.Error()}, nil
	}
	var errs []string
	if ctx := fmt.Sprint(doc.Context); !strings.Contains(ctx, "openvex.dev/ns") {
		errs = append(errs, fmt.Sprintf("@context is not an OpenVEX namespace: %s", common.PyRepr(doc.Context)))
	}
	for _, f := range []struct {
		name string
		ok   bool
	}{{"@id", doc.ID != ""}, {"author", doc.Author != ""}, {"timestamp", doc.Timestamp != ""}, {"statements", len(doc.Statements) > 0}} {
		if !f.ok {
			errs = append(errs, "missing required top-level field: "+f.name)
		}
	}
	seen := map[[2]string]bool{}
	for i, st := range doc.Statements {
		where := fmt.Sprintf("statement[%d]", i)
		cve := strings.TrimSpace(st.Vulnerability.Name)
		switch {
		case cve == "":
			errs = append(errs, where+": no vulnerability.name")
		case !(strings.HasPrefix(cve, "CVE-") || strings.HasPrefix(cve, "GHSA-") || strings.HasPrefix(cve, "GO-")):
			errs = append(errs, fmt.Sprintf("%s: '%s' is not a CVE/GHSA/GO identifier", where, cve))
		}
		if len(st.Products) == 0 {
			errs = append(errs, fmt.Sprintf("%s (%s): no products -- a statement must name what it applies to", where, cve))
		}
		for _, p := range st.Products {
			if p.ID == "" {
				errs = append(errs, fmt.Sprintf("%s (%s): a product has no @id", where, cve))
				continue
			}
			k := [2]string{cve, p.ID}
			if seen[k] {
				errs = append(errs, fmt.Sprintf("%s: duplicate statement for %s on %s", where, cve, p.ID))
			}
			seen[k] = true
		}
		if st.Status != "not_affected" {
			errs = append(errs, fmt.Sprintf("%s (%s): status is %s; only 'not_affected' may appear in a suppression file", where, cve, common.PyRepr(st.Status)))
		}
		if j, _ := st.Justification.(string); !justifications[j] {
			errs = append(errs, fmt.Sprintf("%s (%s): justification %s is not an OpenVEX value", where, cve, common.PyRepr(st.Justification)))
		}
		impact := strings.TrimSpace(st.ImpactStatement)
		if n := len([]rune(impact)); n < minImpact {
			errs = append(errs, fmt.Sprintf("%s (%s): impact_statement is %d chars; needs >= %d explaining what was checked and what it showed", where, cve, n, minImpact))
		} else if !common.ContainsAny(strings.ToLower(impact), evidenceMarkers) {
			errs = append(errs, fmt.Sprintf("%s (%s): impact_statement cites no verification -- say how the claim was checked, not just that it is a false positive", where, cve))
		}
	}
	return errs, &doc
}

// checkVexWiring: the local gate picks the file up by itself, so a workflow that does not
// pass TRIVY_VEX fails only at publish time (grafana, jenkins-inbound-agent and kgateway,
// all on 2026-08-27, each copied from an app without a VEX).
func checkVexWiring(app string) []string {
	wf := filepath.Join(core.ImagesRoot, ".github", "workflows", "build-"+app+".yml")
	b, err := os.ReadFile(wf)
	if err != nil {
		return []string{"no workflow at .github/workflows/build-" + app + ".yml to check"}
	}
	text := string(b)
	var errs []string
	if !strings.Contains(text, "TRIVY_VEX: apps/"+app+"/vex.openvex.json") {
		errs = append(errs, "build-"+app+".yml does not pass TRIVY_VEX -- the VEX clears the LOCAL gate but CI will still fail on the suppressed findings")
	}
	if !strings.Contains(text, "run . vex "+app) {
		errs = append(errs, "build-"+app+".yml does not run `qw vex` -- CI would honour this file without validating it")
	}
	return errs
}

// checkVexStale scans EVERY published version (an app ships several lines; a statement
// stale on one can be live on another) without --vex, at every severity, and lists the
// statements whose CVE none of them carries. It says why when it cannot run.
func checkVexStale(app string, doc *vexDoc) []string {
	b, err := os.ReadFile(filepath.Join(core.ImagesRoot, "catalog.lock.yaml"))
	if err != nil {
		return []string{"stale check skipped: no catalog.lock.yaml"}
	}
	var lock struct {
		Apps map[string]struct {
			Image    string `yaml:"image"`
			Versions []struct {
				Version any    `yaml:"version"`
				Digest  string `yaml:"digest"`
			} `yaml:"versions"`
		} `yaml:"apps"`
	}
	if err := yaml.Unmarshal(b, &lock); err != nil {
		return []string{"stale check skipped: " + err.Error()}
	}
	entry, ok := lock.Apps[app]
	if !ok {
		return []string{"stale check skipped: " + app + " absent from catalog.lock.yaml"}
	}
	found := map[string]bool{}
	var scanned []string
	for _, v := range entry.Versions {
		if v.Digest == "" {
			continue
		}
		out, err := exec.Command("trivy", "image", "--quiet", "--format", "json", "--scanners", "vuln",
			"--detection-priority", "comprehensive", "--severity", "CRITICAL,HIGH,MEDIUM,LOW,UNKNOWN",
			entry.Image+"@"+v.Digest).Output()
		var rep struct {
			Results []struct {
				Vulnerabilities []struct{ VulnerabilityID string }
			}
		}
		if err == nil {
			err = json.Unmarshal(out, &rep)
		}
		if err != nil {
			msg := err.Error()
			return []string{fmt.Sprintf("stale check FAILED on %v: %s", v.Version, msg[:min(110, len(msg))])}
		}
		for _, r := range rep.Results {
			for _, x := range r.Vulnerabilities {
				found[x.VulnerabilityID] = true
			}
		}
		scanned = append(scanned, fmt.Sprint(v.Version))
	}
	if len(scanned) == 0 {
		return []string{"stale check skipped: " + app + " has no published digest"}
	}
	var errs []string
	for _, st := range doc.Statements {
		if c := st.Vulnerability.Name; c != "" && !found[c] {
			errs = append(errs, fmt.Sprintf("stale, in none of %s: %s", strings.Join(scanned, ", "), c))
		}
	}
	return errs
}

func Run(args []string) int {
	stale := false
	var only []string
	for _, a := range args {
		if a == "--stale" {
			stale = true
		} else if !strings.HasPrefix(a, "-") {
			only = append(only, a)
		}
	}
	files, _ := filepath.Glob(filepath.Join(core.ImagesRoot, "apps", "*", "vex.openvex.json"))
	// by app name: whole-path order puts emissary-ingress/ before emissary/ ('-' < '/')
	sort.Slice(files, func(i, j int) bool { return filepath.Dir(files[i]) < filepath.Dir(files[j]) })
	if len(only) > 0 {
		want := map[string]bool{}
		for _, a := range only {
			want[a] = true
		}
		var keep []string
		for _, f := range files {
			if a := filepath.Base(filepath.Dir(f)); want[a] {
				keep = append(keep, f)
				delete(want, a)
			}
		}
		if len(want) > 0 {
			for _, m := range slices.Sorted(maps.Keys(want)) {
				fmt.Printf("!! %s: no apps/%s/vex.openvex.json\n", m, m)
			}
			return 3
		}
		files = keep
	}
	if len(files) == 0 {
		fmt.Println("no VEX files found; nothing to check")
		return 0
	}
	bad := 0
	for _, f := range files {
		app := filepath.Base(filepath.Dir(f))
		errs, doc := checkVexFile(f)
		errs = append(errs, checkVexWiring(app)...)
		if stale && doc != nil {
			errs = append(errs, checkVexStale(app, doc)...)
		}
		if len(errs) > 0 {
			bad++
			fmt.Printf("!! %s\n", app)
			for _, e := range errs {
				fmt.Printf("     %s\n", e)
			}
		} else {
			fmt.Printf("ok %s: %d statement(s)\n", app, len(doc.Statements))
		}
	}
	if bad > 0 {
		fmt.Printf("\n%d VEX file(s) rejected. A VEX entry removes a finding from the 0-CVE gate, so it has to be airtight.\n", bad)
		return 1
	}
	return 0
}
