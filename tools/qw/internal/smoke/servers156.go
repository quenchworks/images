package smoke

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with our postgresql: the image's prisma CLI runs the migrations, the web app registers a
	// user and opens an API session (a wrong password is refused), a saved link to a page on the host is archived by
	// the worker through Wolfi chromium (a screenshot and a PDF land on the shared volume), and the account survives
	// a restart of the web app.
	register("linkwarden", func(t *T) {
		const base = "http://127.0.0.1:18110"
		const pw, dbpw = "Smoke-Linkwarden-2026!", "smoke-db-pass"
		u := t.User("1001")
		page := t.hostStub(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "<!doctype html><html><head><title>Quench smoke page</title></head><body><h1>quench linkwarden smoke</h1></body></html>")
		}))
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		pg := t.Name("pg")
		t.Cleanup(func() { docker("rm", "-f", pg) })
		t.Docker("run", "-d", "--name", pg, "--network", net, "-e", "POSTGRES_PASSWORD="+dbpw, "-e", "POSTGRES_DB=linkwarden", "ghcr.io/quenchworks/images/postgresql:18.7")
		t.waitCount(pg, "ready to accept connections", 2, 60)
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		env := []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/data/data",
			"-e", "DATABASE_URL=postgresql://postgres:" + dbpw + "@" + pg + ":5432/linkwarden",
			"-e", "NEXTAUTH_SECRET=quench-smoke-secret-0123456789abcdef", "-e", "NEXTAUTH_URL=" + base + "/api/v1/auth"}
		// migrations: the image's prisma CLI, as a one-shot container
		mig := t.Name("migrate")
		t.Cleanup(func() { docker("rm", "-f", mig) })
		out, err := docker(append(append([]string{"run", "--name", mig, "--user", "1001", "--entrypoint", "/usr/bin/node"}, env...),
			t.Image, "/data/node_modules/prisma/build/index.js", "migrate", "deploy", "--schema", "/data/packages/prisma/schema.prisma")...)
		if err != nil || !strings.Contains(out, "migrations") {
			t.Fatalf("migrate deploy: %v\n%s", err, out)
		}
		web := func(name string) {
			t.Start(name, append(append([]string{}, env...), "-p", "127.0.0.1:18110:3000"))
			t.WaitHTTP(base+"/login", 120, name)
		}
		name := t.Name("")
		web(name)
		worker := t.Name("worker")
		t.Start(worker, append(append(append([]string{}, env...), hostStubFlag...), "--workdir", "/data/apps/worker", "--shm-size", "256m",
			"-e", "ALLOW_PRIVATE_NETWORK_ACCESS=true", "--entrypoint", "/usr/bin/node"), "/data/node_modules/tsx/dist/cli.mjs", "worker.ts")
		j := map[string]string{"Content-Type": "application/json"}
		if c, b := DoH("POST", base+"/api/v1/users", `{"name":"Smoke","username":"smoke","password":"`+pw+`"}`, j); c/100 != 2 {
			t.Fatalf("register: %d %s\n%s", c, b, t.Logs(name))
		}
		session := func(pass string) (int, string) {
			c, b := DoH("POST", base+"/api/v1/session", `{"username":"smoke","password":"`+pass+`","sessionName":"smoke"}`, j)
			var r struct {
				Response struct{ Token string } `json:"response"`
			}
			json.Unmarshal([]byte(b), &r)
			return c, r.Response.Token
		}
		c, tok := session(pw)
		if c != http.StatusOK || tok == "" {
			t.Fatalf("session: %d", c)
		}
		auth := map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"}
		c, b := DoH("POST", base+"/api/v1/links", `{"url":"`+page+`/smoke","name":"Quench smoke"}`, auth)
		var created struct {
			Response struct{ ID int } `json:"response"`
		}
		if c/100 != 2 || json.Unmarshal([]byte(b), &created) != nil || created.Response.ID == 0 {
			t.Fatalf("save a link: %d %s", c, b)
		}
		archived := false
		for i := 0; i < 90 && !archived; i++ {
			_, b := DoH("GET", fmt.Sprintf("%s/api/v1/links/%d", base, created.Response.ID), "", auth)
			var l struct {
				Response struct{ Image, Pdf string } `json:"response"`
			}
			json.Unmarshal([]byte(b), &l)
			archived = l.Response.Image != "" && l.Response.Image != "unavailable" && l.Response.Pdf != "" && l.Response.Pdf != "unavailable"
			if !archived {
				time.Sleep(2 * time.Second)
			}
		}
		if !archived {
			t.Fatalf("the worker did not archive the page (screenshot and PDF)\n%s", t.Logs(worker))
		}
		docker("rm", "-f", name)
		web(t.Name("again"))
		if c, tok := session(pw); c != http.StatusOK || tok == "" {
			t.Fatalf("session after a restart: %d", c)
		}
		if c, tok := session("wrong-password-1"); c == http.StatusOK || tok != "" {
			t.Fatalf("a wrong password got a session (%d)", c)
		}
		t.Logf("smoke test passed (linkwarden %s: read-only boot, prisma migrations, user registered and API session opened, page archived by the worker with chromium (screenshot and PDF), kept across a restart, wrong password refused; user %s)", t.Want, u)
	})
}
