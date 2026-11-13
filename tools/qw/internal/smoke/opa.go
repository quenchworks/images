package smoke

func init() {
	register("opa", func(t *T) {
		name := t.Name("")
		// the default command runs the policy server on :8181
		t.Start(name, []string{"-p", "127.0.0.1:8181:8181"})
		t.WaitHTTP("http://127.0.0.1:8181/health", 30, name)
		t.Logf("healthy; evaluating a trivial policy via the Data API")
		if code, body := Get("http://127.0.0.1:8181/v1/data"); code != 200 {
			t.Fatalf("data API answered %d %s\n%s", code, body, t.Logs(name))
		}
		// --entrypoint: the image's entrypoint is "opa run --server", which would read
		// "version" as a bundle path
		ver := t.Match(t.RunEntry("/usr/bin/opa", "version"), `(?m)^Version:\s*(\S+)`, "opa version")
		t.Logf("reported version: %s", ver)
		if ver == "0.0.0" || regexpDev.MatchString(ver) {
			t.Fatalf("version not stamped: '%s'", ver)
		}
		t.WantVersion(ver)
		u := t.User("1001")
		t.Logf("smoke test passed (version %s, nonroot user: %s)", ver, u)
	})
}
