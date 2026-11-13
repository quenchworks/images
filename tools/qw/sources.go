package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var httpc = &http.Client{Timeout: 30 * time.Second}

// gh runs one `gh api` call and FAILS when the call fails: an empty answer from a
// rate-limited call would otherwise read as "no stable releases" (openbao, 2026-09-18).
func gh(path, jq string) ([]string, error) {
	out, err := exec.Command("gh", "api", path, "-q", jq).Output()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			msg = strings.TrimSpace(string(ee.Stderr))
			if len(msg) > 300 {
				msg = msg[:300]
			}
		}
		return nil, fmt.Errorf("gh api %s failed: %s", path, msg)
	}
	var vs []string
	for _, t := range strings.Fields(string(out)) {
		vs = append(vs, strings.TrimLeft(t, "v"))
	}
	return vs, nil
}

// github: stable, non-draft release tags. The name filter backs up the prerelease
// flag, which upstreams forget (kgateway v2.5.0-beta.1 had prerelease=false).
func github(repo string) ([]string, error) {
	return gh("repos/"+repo+"/releases?per_page=40",
		`.[]|select(.prerelease==false and .draft==false)|.tag_name|select(test("[-.](alpha|beta|rc|pre|preview)[.0-9]*$"; "i")|not)`)
}

// githubAssetReleases: release tags that ship a downloadable asset (prereleases included).
func githubAssetReleases(repo string) ([]string, error) {
	return gh("repos/"+repo+"/releases?per_page=40", `.[]|select(.draft==false and (.assets|length)>0)|.tag_name`)
}

// githubTags: every git tag via ls-remote. The tags API pages in name order, so
// prerelease tags can push every stable one off page one (cockroachdb, 2026-11-02).
func githubTags(repo string) ([]string, error) {
	out, err := exec.Command("git", "ls-remote", "--tags", "--refs", "https://github.com/"+repo).Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-remote %s failed: %v", repo, err)
	}
	var vs []string
	for _, l := range strings.Split(string(out), "\n") {
		if _, t, ok := strings.Cut(l, "refs/tags/"); ok {
			vs = append(vs, strings.TrimLeft(t, "v"))
		}
	}
	return vs, nil
}

func fetch(url string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	// Wikimedia (and others) refuse a default client agent with a 403.
	req.Header.Set("User-Agent", "quenchworks-update-check (info@quench-works.com)")
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func jsonGet(url string, v any) error {
	b, err := fetch(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func npm(pkg string) ([]string, error) {
	var d struct{ Versions map[string]json.RawMessage }
	if err := jsonGet("https://registry.npmjs.org/"+pkg, &d); err != nil {
		return nil, err
	}
	return keys(d.Versions), nil
}

func pypi(pkg string) ([]string, error) {
	var d struct{ Releases map[string]json.RawMessage }
	if err := jsonGet("https://pypi.org/pypi/"+pkg+"/json", &d); err != nil {
		return nil, err
	}
	return keys(d.Releases), nil
}

// scrape returns every group-1 match of pattern in the page (all matches if no group).
func scrape(url, pattern string) ([]string, error) {
	b, err := fetch(url)
	if err != nil {
		return nil, err
	}
	var vs []string
	for _, m := range regexp.MustCompile(pattern).FindAllStringSubmatch(string(b), -1) {
		vs = append(vs, m[len(m)-1])
	}
	return vs, nil
}

// wolfi: X.Y.Z versions of one flat Wolfi apk (-rN dropped), newest 40.
func wolfi(pkg string) ([]string, error) {
	idx, err := loadWolfi()
	if err != nil {
		return nil, err
	}
	var vs []string
	for i, v := range bestPerRelease(idx[pkg]) {
		if i == 40 {
			break
		}
		x, _, _ := strings.Cut(v, "-r")
		vs = append(vs, x)
	}
	return vs, nil
}

// wolfiMatch: {package: newest X.Y.Z} for every Wolfi apk whose name matches pattern.
func wolfiMatch(pattern string) (map[string]string, error) {
	idx, err := loadWolfi()
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(pattern)
	out := map[string]string{}
	for p, vs := range idx {
		if re.MatchString(p) {
			x, _, _ := strings.Cut(bestPerRelease(vs)[0], "-r")
			out[p] = x
		}
	}
	return out, nil
}
