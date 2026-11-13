// Package smoke holds the Go smoke tests for built images: one function per app, run as
// `qw smoke <app> <image-ref> [version]`. An app's apps/<app>/test.sh is a one-line call into
// here once its test is ported, so build-image.sh and the build workflows need no change.
package smoke

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// tests maps an app slug to its smoke test. Each app file registers itself in an init.
var tests = map[string]func(*T){}

func register(app string, f func(*T)) {
	if _, dup := tests[app]; dup {
		panic("smoke test registered twice: " + app)
	}
	tests[app] = f
}

// Apps lists the apps with a Go smoke test, sorted.
func Apps() []string {
	out := make([]string, 0, len(tests))
	for a := range tests {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// T is one smoke test run. Its check helpers stop the test on failure, as testing.T's do.
type T struct {
	App, Image, Want string
	Args             []string // arguments after the version, for tests that take more than one
	name             string
	failed           bool
	cleanup          []func()
}

// Fatalf records a failure and ends the test (deferred cleanups still run).
func (t *T) Fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	t.failed = true
	runtime.Goexit()
}

// Logf prints progress to stdout.
func (t *T) Logf(format string, a ...any) { fmt.Printf(format+"\n", a...) }

// Cleanup runs f after the test, last registered first.
func (t *T) Cleanup(f func()) { t.cleanup = append(t.cleanup, f) }

// Name is a container name unique to this run.
func (t *T) Name(suffix string) string {
	if suffix == "" {
		return t.name
	}
	return t.name + "-" + suffix
}

// docker runs a docker command and returns combined output and its error.
func docker(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// Docker runs a docker command and fails the test if it fails.
func (t *T) Docker(args ...string) string {
	out, err := docker(args...)
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// Run runs the image once with args (docker run --rm IMAGE args...) and returns its output.
func (t *T) Run(args ...string) string {
	return t.Docker(append([]string{"run", "--rm", t.Image}, args...)...)
}

// RunEntry runs the image once with another entrypoint.
func (t *T) RunEntry(entry string, args ...string) string {
	return t.Docker(append([]string{"run", "--rm", "--entrypoint", entry, t.Image}, args...)...)
}

// RunAny runs the image once and returns its output whatever the exit code (bash "|| true").
func (t *T) RunAny(args ...string) string { out, _ := t.TryRun(args...); return out }

// RunEntryAny is RunEntry whatever the exit code.
func (t *T) RunEntryAny(entry string, args ...string) string {
	out, _ := docker(append([]string{"run", "--rm", "--entrypoint", entry, t.Image}, args...)...)
	return out
}

// TryRun is Run that returns the error instead of failing (for commands expected to fail).
func (t *T) TryRun(args ...string) (string, error) {
	return docker(append([]string{"run", "--rm", t.Image}, args...)...)
}

// Start runs the image detached as name with extra docker run flags before the image and
// args after it, and removes the container when the test ends.
func (t *T) Start(name string, flags []string, args ...string) {
	t.Cleanup(func() { docker("rm", "-f", name) })
	a := append([]string{"run", "-d", "--name", name}, flags...)
	t.Docker(append(append(a, t.Image), args...)...)
}

// Logs returns a container's combined logs.
func (t *T) Logs(name string) string { out, _ := docker("logs", name); return out }

// User fails unless the image runs as want (docker inspect Config.User).
func (t *T) User(want string) string {
	u := t.Docker("inspect", t.Image, "--format", "{{.Config.User}}")
	if u != want {
		t.Fatalf("expected user %s, got '%s'", want, u)
	}
	return u
}

// Match fails unless s matches the regexp re; it returns the first submatch (or the match).
func (t *T) Match(s, re, what string) string {
	m := regexp.MustCompile(re).FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("%s: no match for %s in:\n%s", what, re, s)
	}
	return m[len(m)-1]
}

// Contains fails unless s contains sub.
func (t *T) Contains(s, sub, what string) {
	if !strings.Contains(s, sub) {
		t.Fatalf("%s: %q not found in:\n%s", what, sub, s)
	}
}

// WantVersion fails if the test was given a version and s does not contain it.
func (t *T) WantVersion(s string) {
	if t.Want != "" && !strings.Contains(s, t.Want) {
		t.Fatalf("expected version %s in:\n%s", t.Want, s)
	}
}

// WantVersionV is WantVersion for tools that print the version as v<version>.
func (t *T) WantVersionV(s string) {
	if t.Want != "" && !strings.Contains(s, "v"+t.Want) {
		t.Fatalf("expected v%s in:\n%s", t.Want, s)
	}
}

// ContainsFold is Contains, case-insensitive (grep -qi).
func (t *T) ContainsFold(s, sub, what string) {
	if !strings.Contains(strings.ToLower(s), strings.ToLower(sub)) {
		t.Fatalf("%s: %q not found in:\n%s", what, sub, s)
	}
}

// RunFlags runs the image once with docker run flags before the image; it returns the
// output and the error instead of failing.
func (t *T) RunFlags(flags []string, args ...string) (string, error) {
	return docker(append(append(append([]string{"run", "--rm"}, flags...), t.Image), args...)...)
}

// LabelVersion fails if the test was given a version and the image's
// org.opencontainers.image.version label differs; it returns the label.
func (t *T) LabelVersion() string {
	l := t.Docker("inspect", t.Image, "--format", `{{index .Config.Labels "org.opencontainers.image.version"}}`)
	if t.Want != "" && l != t.Want {
		t.Fatalf("image label %s, expected %s", l, t.Want)
	}
	return l
}

// RunInput runs the image once with input on stdin (docker run -i); it returns the output
// and the error instead of failing.
func (t *T) RunInput(input string, args ...string) (string, error) {
	return t.RunInputFlags(nil, input, args...)
}

// RunInputFlags is RunInput with docker run flags before the image.
func (t *T) RunInputFlags(flags []string, input string, args ...string) (string, error) {
	c := exec.Command("docker", append(append(append([]string{"run", "--rm", "-i"}, flags...), t.Image), args...)...)
	c.Stdin = strings.NewReader(input)
	out, err := c.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// Entries returns the tar headers of the image's files whose path starts with prefix (no
// leading "/"), read from a docker export, so it works on images without a shell.
func (t *T) Entries(prefix string) []*tar.Header {
	id := t.Docker("create", t.Image)
	defer docker("rm", id)
	c := exec.Command("docker", "export", id)
	r, err := c.StdoutPipe()
	if err != nil {
		t.Fatalf("docker export: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("docker export: %v", err)
	}
	var out []*tar.Header
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if strings.HasPrefix(h.Name, prefix) {
			out = append(out, h)
		}
	}
	c.Wait()
	return out
}

// Files lists the paths in the image that start with prefix (see Entries).
func (t *T) Files(prefix string) []string {
	var out []string
	for _, h := range t.Entries(prefix) {
		out = append(out, h.Name)
	}
	return out
}

// WaitLog polls a container's logs once a second until they contain sub, up to tries.
func (t *T) WaitLog(name, sub string, tries int) {
	for i := 0; i < tries; i++ {
		if strings.Contains(t.Logs(name), sub) {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%q never appeared in the logs of %s:\n%s", sub, name, t.Logs(name))
}

// RunCode runs the image once with docker run flags before the image and returns the
// output and the exit code (-1 when docker itself failed to report one).
func (t *T) RunCode(flags []string, args ...string) (string, int) {
	out, err := t.RunFlags(flags, args...)
	if err == nil {
		return out, 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out, ee.ExitCode()
	}
	return out, -1
}

// runHost runs a program on the host (not in a container) and returns its combined output.
func runHost(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// RunAnyFor runs the image (docker run flags, then args) for at most d, like bash
// `timeout d docker run`, removes the container, and returns its output and exit code
// (124 when it was still running at the deadline).
func (t *T) RunAnyFor(d time.Duration, flags []string, args ...string) (string, int) {
	name := fmt.Sprintf("%s-for-%d", t.name, time.Now().UnixNano())
	defer docker("rm", "-f", name)
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	a := append(append(append([]string{"run", "--rm", "--name", name}, flags...), t.Image), args...)
	out, err := exec.CommandContext(ctx, "docker", a...).CombinedOutput()
	o := strings.TrimRight(string(out), "\n")
	if ctx.Err() != nil {
		return o, 124
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return o, ee.ExitCode()
	}
	if err != nil {
		return o, -1
	}
	return o, 0
}

// TempDir makes a world-readable temp dir (under TMPDIR, which build-image.sh points at a
// path Docker can mount) and removes it when the test ends.
func (t *T) TempDir() string {
	d, err := os.MkdirTemp("", "quench-smoke-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	os.Chmod(d, 0o777)
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

// DeadKubeconfig writes a kubeconfig whose API server (127.0.0.1:1) never answers, for
// controllers that must start, dial it, and stop or retry without crashing.
func (t *T) DeadKubeconfig() string {
	p := filepath.Join(t.TempDir(), "kubeconfig")
	os.WriteFile(p, []byte(`apiVersion: v1
kind: Config
clusters: [{name: none, cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}}]
users: [{name: none, user: {token: none}}]
contexts: [{name: none, context: {cluster: none, user: none}}]
current-context: none
`), 0o644)
	return p
}

// WaitTCP polls until every addr accepts a TCP connection, once a second up to tries.
func (t *T) WaitTCP(tries int, container string, addrs ...string) {
	for i := 0; i < tries; i++ {
		ok := true
		for _, a := range addrs {
			c, err := net.DialTimeout("tcp", a, 2*time.Second)
			if err != nil {
				ok = false
				break
			}
			c.Close()
		}
		if ok {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("ports %v never opened\n--- logs:\n%s", addrs, t.Logs(container))
}

// randFrom returns n characters drawn at random from alphabet.
func randFrom(alphabet string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rand.IntN(len(alphabet))]
	}
	return string(b)
}

// regexpDev matches unstamped development versions.
var regexpDev = regexp.MustCompile(`dev`)

var httpc = &http.Client{Timeout: 10 * time.Second}

// Get does one HTTP GET and returns the status code and body (code 0 on a transport error).
func Get(url string) (int, string) {
	r, err := httpc.Get(url)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

// Post does one HTTP POST with a content type and body.
func Post(url, ctype, body string) (int, string) {
	r, err := httpc.Post(url, ctype, bytes.NewBufferString(body))
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

// Do sends one HTTP request with a body (empty for none) and returns the status and body.
func Do(method, url, body string) (int, string) { return DoH(method, url, body, nil) }

// DoH is Do with request headers.
func DoH(method, url, body string, hdr map[string]string) (int, string) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	r, err := httpc.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

// WaitHTTP polls url once a second until it answers 2xx, up to tries; on timeout it fails
// with the container's logs.
func (t *T) WaitHTTP(url string, tries int, container string) string {
	var code int
	var body string
	for i := 0; i < tries; i++ {
		if code, body = Get(url); code >= 200 && code < 300 {
			return body
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%s never answered 2xx (last: %d %s)\n--- logs:\n%s", url, code, body, t.Logs(container))
	return ""
}

// Run executes app's smoke test against image and returns the exit code.
func Run(args []string) int {
	if len(args) == 1 && args[0] == "--list" {
		fmt.Println(strings.Join(Apps(), "\n"))
		return 0
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: qw smoke <app> <image-ref> [version] | qw smoke --list")
		return 2
	}
	f, ok := tests[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "smoke: no Go smoke test for %q\n", args[0])
		return 2
	}
	t := &T{App: args[0], Image: args[1], name: fmt.Sprintf("quench-%s-smoke-%d", args[0], os.Getpid())}
	if len(args) > 2 {
		t.Want, t.Args = args[2], args[3:]
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			for i := len(t.cleanup) - 1; i >= 0; i-- {
				t.cleanup[i]()
			}
		}()
		f(t)
	}()
	<-done
	if t.failed {
		return 1
	}
	return 0
}
