package smoke

import "strings"

func init() {
	// On a read-only rootfs: nginx serves the draw.io page and its app bundle at the expected version, and
	// nothing of the war's WEB-INF.
	register("drawio", func(t *T) {
		const base = "http://127.0.0.1:18083"
		u := t.User("1001")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-p", "127.0.0.1:18083:8080"})
		page := t.WaitHTTP(base+"/", 60, name)
		t.Contains(page, "<title>Flowchart Maker", "the page")
		code, app := restDo("GET", base+"/js/app.min.js", "", "", "")
		if code != 200 || !strings.Contains(app, `EditorUi.VERSION="`+t.Want+`"`) {
			t.Fatalf("/js/app.min.js answered %d without version %s\n%s", code, t.Want, t.Logs(name))
		}
		for _, p := range []string{"/js/viewer-static.min.js", "/styles/grapheditor.css"} {
			if code, _ := restDo("GET", base+p, "", "", ""); code != 200 {
				t.Fatalf("%s answered %d", p, code)
			}
		}
		if code, _ := restDo("GET", base+"/WEB-INF/web.xml", "", "", ""); code != 404 {
			t.Fatalf("/WEB-INF/web.xml answered %d, want 404", code)
		}
		t.Logf("smoke test passed (drawio %s: read-only boot, page and app bundle served by nginx, WEB-INF absent; user %s)", t.Want, u)
	})
}
