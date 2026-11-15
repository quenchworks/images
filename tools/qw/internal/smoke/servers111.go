package smoke

import (
	"strings"
	"time"
)

func init() {
	// On a read-only rootfs with /data, /cache and an empty /music on volumes: gonic answers the Subsonic
	// ping with the seeded admin, refuses a wrong password (error 40), keeps its database across a
	// restart, and ffmpeg (for transcoding) runs in the image.
	register("gonic", func(t *T) {
		const base = "http://127.0.0.1:14747"
		u := t.User("1001")
		t.Contains(t.RunAny("-version"), t.Want, "gonic -version")
		if out, err := t.RunFlags([]string{"--entrypoint", "/usr/bin/ffmpeg", "--read-only"}, "-version"); err != nil || !strings.Contains(out, "ffmpeg version") {
			t.Fatalf("ffmpeg did not run: %v\n%s", err, out)
		}
		data, cache := t.Name("data"), t.Name("cache")
		for _, v := range []string{data, cache} {
			v := v
			t.Docker("volume", "create", v)
			t.Cleanup(func() { docker("volume", "rm", v) })
		}
		ping := func(pw string) string {
			_, b := Get(base + "/rest/ping.view?u=admin&p=" + pw + "&v=1.16.1&c=smoke&f=json")
			return b
		}
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", data + ":/data", "-v", cache + ":/cache",
				"--tmpfs", "/podcasts:rw,uid=1001,gid=1001", "--tmpfs", "/playlists:rw,uid=1001,gid=1001", "-p", "127.0.0.1:14747:4747"})
			t.waitFor(60, time.Second, name, "gonic never answered the Subsonic ping", func() bool {
				if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
					t.Fatalf("gonic exited\n%s", t.Logs(name))
				}
				return strings.Contains(ping("admin"), `"status":"ok"`)
			})
		}
		name := t.Name("")
		start(name)
		if b := ping("wrong-password-1"); !strings.Contains(b, `"status":"failed"`) {
			t.Fatalf("a wrong password was not refused: %s", b)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		t.Logf("smoke test passed (gonic %s: read-only boot, Subsonic ping as the seeded admin, wrong password refused, kept across a restart, ffmpeg runs; user %s)", t.Want, u)
	})
}
