package main

// qw gen-catalog writes catalog.lock.yaml: the published state of the GHCR catalog. For
// every app in catalog.yaml it lists the package's version tags (digest + publish date),
// keeps the ones build.conf declares, and merges in the editorial fields. --meta also
// inspects each unique digest for its real arches, compressed amd64 size and layer count.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const org = "quenchworks"

// every image is built amd64+arm64; per-digest arches only come from --meta
var defaultArches = []string{"amd64", "arm64"}

var editorialFields = []string{"category", "summary", "source", "upstream", "license", "tier", "status", "cleanAlternative", "knownIssue"}

type lockVersion struct {
	Version   string  `yaml:"version"`
	Digest    string  `yaml:"digest"`
	Published string  `yaml:"published"`
	Size      *string `yaml:"size,omitempty"`
	Layers    *int    `yaml:"layers,omitempty"`
}

func human(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KB", "MB"} {
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.1f GB", f)
}

// isVersionTag: a release tag, not a registry artifact (cosign sig, scan, arch leg).
func isVersionTag(t string) bool {
	return t != "" && t[0] >= '0' && t[0] <= '9' && !strings.HasPrefix(t, "sha256-") &&
		!strings.HasPrefix(t, "scan-") && !strings.HasSuffix(t, "-amd64") && !strings.HasSuffix(t, "-arm64")
}

// isFullVersion: 8.0.127 or 2026.04.0, not the moving aliases `8` / `1.26`.
func isFullVersion(t string) bool { return isVersionTag(t) && strings.Count(t, ".") >= 2 }

func buildVersions(slug string) map[string]bool {
	out := map[string]bool{}
	conf := filepath.Join(imagesRoot, "apps", slug, "build.conf")
	if _, err := os.Stat(conf); err != nil {
		return out
	}
	b, _ := exec.Command("bash", "-c", `source "`+conf+`"; printf "%s\n" "${VERSIONS[@]}"`).Output()
	for _, v := range strings.Fields(string(b)) {
		out[v] = true
	}
	return out
}

type pkgVersion struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	Metadata  struct {
		Container struct {
			Tags []string `json:"tags"`
		} `json:"container"`
	} `json:"metadata"`
}

// versionsOf lists one package's published tags, newest first. ONLY a 404 means "no
// package yet": any other failure used to become an empty list too, and a transient one
// once delisted dragonfly's three published versions. A loud failure costs a re-run.
func versionsOf(slug string) ([]lockVersion, error) {
	out, err := exec.Command("gh", "api", "--paginate",
		"/orgs/"+org+"/packages/container/images%2F"+slug+"/versions?per_page=100").CombinedOutput()
	if err != nil {
		if s := string(out); strings.Contains(s, "HTTP 404") || strings.Contains(s, "Not Found") {
			return nil, nil
		}
		msg := strings.TrimSpace(string(out))
		return nil, fmt.Errorf("listing versions for images/%s FAILED (not a 404, so the package may well exist -- refusing to report it as unpublished): %s", slug, msg[:min(400, len(msg))])
	}
	// --paginate concatenates page arrays as `][`
	var raw []pkgVersion
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(out), "][", ",")), &raw); err != nil {
		return nil, err
	}
	type pub struct{ digest, created string }
	published := map[string]pub{}
	var order []string
	for _, v := range raw {
		for _, t := range v.Metadata.Container.Tags {
			if _, ok := published[t]; !ok && isVersionTag(t) {
				published[t] = pub{v.Name, v.CreatedAt}
				order = append(order, t)
			}
		}
	}
	// the exact build.conf VERSIONS; if none are published yet (bumped, not rebuilt),
	// the published FULL versions, never the bare major aliases and never empty
	allowed := buildVersions(slug)
	var keep []string
	for _, t := range order {
		if allowed[t] {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		for _, t := range order {
			if isFullVersion(t) {
				keep = append(keep, t)
			}
		}
	}
	var vs []lockVersion
	for _, t := range keep {
		vs = append(vs, lockVersion{Version: t, Digest: published[t].digest, Published: published[t].created})
	}
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].Published > vs[j].Published })
	return vs, nil
}

type imageMeta struct {
	arches []string
	size   *string
	layers *int
}

var (
	metaMu    sync.Mutex
	metaCache = map[string]*imageMeta{}
)

func rawManifest(ref string) map[string]any {
	out, err := exec.Command("docker", "buildx", "imagetools", "inspect", ref, "--raw").Output()
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(out, &m) != nil {
		return nil
	}
	return m
}

// metaOf: real arches + compressed amd64 size + layers, cached per digest.
func metaOf(repo, digest string) *imageMeta {
	key := repo + "@" + digest
	metaMu.Lock()
	if m, ok := metaCache[key]; ok {
		metaMu.Unlock()
		return m
	}
	metaMu.Unlock()
	m := &imageMeta{}
	if idx := rawManifest(key); idx != nil {
		var amd map[string]any
		set := map[string]bool{}
		for _, x := range asList(idx["manifests"]) {
			mm, _ := x.(map[string]any)
			arch := str(path(mm, "platform", "architecture"))
			if arch == "" || arch == "unknown" {
				continue
			}
			set[arch] = true
			if arch == "amd64" && amd == nil {
				amd = mm
			}
		}
		m.arches = sortedKeys(set)
		if len(m.arches) == 0 {
			m.arches = defaultArches
		}
		pm := idx // single-arch: the index IS the manifest
		if amd != nil {
			pm = rawManifest(repo + "@" + str(amd["digest"]))
		}
		if pm != nil {
			if ls, ok := pm["layers"].([]any); ok {
				var total int64
				for _, l := range ls {
					total += int64(num(path(l, "size")))
				}
				total += int64(num(path(pm, "config", "size")))
				h := human(total)
				n := len(ls)
				m.size, m.layers = &h, &n
			}
		}
	}
	metaMu.Lock()
	metaCache[key] = m
	metaMu.Unlock()
	return m
}

func asList(d any) []any { a, _ := d.([]any); return a }
func num(d any) float64  { f, _ := d.(float64); return f }

// parallel runs f(0..n-1) on `workers` goroutines.
func parallel(n, workers int, f func(int)) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() { defer wg.Done(); f(i); <-sem }()
	}
	wg.Wait()
}

func cmdGenCatalog(args []string) int {
	withMeta := false
	out := filepath.Join(imagesRoot, "catalog.lock.yaml")
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--meta":
			withMeta = true
		case "-o":
			i++
			out = args[i]
		}
	}
	b, err := os.ReadFile(filepath.Join(imagesRoot, "catalog.yaml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var cur struct {
		Catalog []map[string]any `yaml:"catalog"`
	}
	if err := yaml.Unmarshal(b, &cur); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// status: blocked == delisted: kept out of the lock, images.json and the nightly scan
	var rows []map[string]any
	for _, r := range cur.Catalog {
		if r["status"] != "blocked" {
			rows = append(rows, r)
		}
	}
	repo := func(slug string) string { return "ghcr.io/" + org + "/images/" + slug }

	fmt.Fprintf(os.Stderr, "• phase 1/3: listing published tags for %d apps\n", len(rows))
	listed := make([][]lockVersion, len(rows))
	errs := make([]error, len(rows))
	parallel(len(rows), 16, func(i int) { listed[i], errs[i] = versionsOf(fmt.Sprint(rows[i]["name"])) })
	for _, e := range errs {
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
	}
	if withMeta {
		var pairs [][2]string
		seen := map[string]bool{}
		for i, vs := range listed {
			for _, v := range vs {
				if k := repo(fmt.Sprint(rows[i]["name"])) + "@" + v.Digest; !seen[k] {
					seen[k] = true
					pairs = append(pairs, [2]string{repo(fmt.Sprint(rows[i]["name"])), v.Digest})
				}
			}
		}
		fmt.Fprintf(os.Stderr, "• phase 2/3: inspecting %d unique image digests\n", len(pairs))
		parallel(len(pairs), 32, func(i int) { metaOf(pairs[i][0], pairs[i][1]) })
	}
	fmt.Fprintln(os.Stderr, "• phase 3/3: assembling lock")
	apps := &yaml.Node{Kind: yaml.MappingNode}
	total := 0
	for i, r := range rows {
		name := fmt.Sprint(r["name"])
		vs := listed[i]
		arches := defaultArches
		if withMeta {
			for j := range vs {
				m := metaOf(repo(name), vs[j].Digest)
				vs[j].Size, vs[j].Layers = m.size, m.layers
			}
			if len(vs) > 0 {
				if a := metaOf(repo(name), vs[0].Digest).arches; len(a) > 0 {
					arches = a
				}
			}
		}
		if vs == nil {
			vs = []lockVersion{}
		}
		app := &yaml.Node{Kind: yaml.MappingNode}
		add := func(k string, v any) {
			var n yaml.Node
			_ = n.Encode(v)
			app.Content = append(app.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &n)
		}
		add("image", repo(name))
		for _, k := range editorialFields {
			if v, ok := r[k]; ok && v != nil {
				add(k, v)
			}
		}
		add("arches", arches)
		add("versions", vs)
		apps.Content = append(apps.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: name}, app)
		total += len(vs)
	}
	doc := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "registry"}, {Kind: yaml.ScalarNode, Value: "ghcr.io/" + org + "/images"},
		{Kind: yaml.ScalarNode, Value: "apps"}, apps,
	}}
	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer f.Close()
	fmt.Fprint(f, "# GENERATED by qw gen-catalog (tools/qw) -- do not edit by hand.\n# Published state of the GHCR catalog (run it to refresh).\n")
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	enc.Close()
	fmt.Printf("wrote %s: %d apps, %d published versions\n", out, len(rows), total)
	return 0
}
