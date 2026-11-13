package probe

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// FakeAPI serves canned JSON responses, the slice of a Kubernetes API server a smoke test
// needs to drive a controller without a cluster. Anything unrouted gets a NotFound Status.
//
//	--route  "METHOD /path=STATUS:file"  respond with file's content (repeatable)
//	--record "METHOD /path=file"         also save that request's body to file (repeatable)
//	--log file                           append each request path, one per line
//	--cert/--key                         serve TLS
//	--port-file file                     write the bound port (for --addr 127.0.0.1:0)
func FakeAPI(args []string) int {
	fs := flag.NewFlagSet("fakeapi", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:0", "listen address")
	cert := fs.String("cert", "", "TLS certificate (PEM)")
	key := fs.String("key", "", "TLS key (PEM)")
	logf := fs.String("log", "", "append each request path to this file")
	portFile := fs.String("port-file", "", "write the bound port here")
	var routes, records multi
	fs.Var(&routes, "route", `"METHOD /path=STATUS:file"`)
	fs.Var(&records, "record", `"METHOD /path=file"`)
	if fs.Parse(args) != nil {
		return 2
	}
	type resp struct {
		code int
		body []byte
	}
	table := map[string]resp{}
	for _, r := range routes {
		k, v, ok := strings.Cut(r, "=")
		code, file, ok2 := strings.Cut(v, ":")
		n, err := strconv.Atoi(code)
		if !ok || !ok2 || err != nil {
			return fail("bad --route %q", r)
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return fail("route %q: %v", r, err)
		}
		table[k] = resp{n, b}
	}
	rec := map[string]string{}
	for _, r := range records {
		k, v, ok := strings.Cut(r, "=")
		if !ok {
			return fail("bad --record %q", r)
		}
		rec[k] = v
	}
	var mu sync.Mutex
	var logw io.Writer = io.Discard
	if *logf != "" {
		f, err := os.OpenFile(*logf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fail("log: %v", err)
		}
		defer f.Close()
		logw = f
	}
	notFound := []byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","message":"fakeapi","code":404}`)
	h := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		k := req.Method + " " + req.URL.Path
		body, _ := io.ReadAll(req.Body)
		mu.Lock()
		fmt.Fprintln(logw, req.URL.Path)
		if f, ok := rec[k]; ok {
			os.WriteFile(f, body, 0o644)
		}
		mu.Unlock()
		r, ok := table[k]
		if !ok {
			r = resp{http.StatusNotFound, notFound}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(r.body)))
		w.WriteHeader(r.code)
		w.Write(r.body)
	})
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fail("listen: %v", err)
	}
	if *portFile != "" {
		os.WriteFile(*portFile, []byte(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)), 0o644)
	}
	if *cert != "" {
		err = http.ServeTLS(ln, h, *cert, *key)
	} else {
		err = http.Serve(ln, h)
	}
	return fail("serve: %v", err)
}
