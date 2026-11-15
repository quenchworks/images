package smoke

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// grocyLogin posts the login form: true when Grocy redirects to / (a failed login redirects to
// /login?invalid=true), plus the body of /api/system/info read with the session. With addProduct it
// also creates a location, a quantity unit and a product through the API, the writes SQLite 3.54.1 breaks.
func grocyLogin(base, user, pass string, addProduct ...bool) (bool, string) {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.PostForm(base+"/login", url.Values{"username": {user}, "password": {pass}})
	if err != nil {
		return false, err.Error()
	}
	r.Body.Close()
	if loc := r.Header.Get("Location"); r.StatusCode/100 != 3 || strings.Contains(loc, "invalid") {
		return false, r.Status + " " + loc
	}
	s, err := c.Get(base + "/api/system/info")
	if err != nil {
		return false, err.Error()
	}
	defer s.Body.Close()
	b, _ := io.ReadAll(s.Body)
	if s.StatusCode != http.StatusOK || len(addProduct) == 0 {
		return s.StatusCode == http.StatusOK, string(b)
	}
	post := func(obj, body string) (int, string) {
		r, err := c.Post(base+"/api/objects/"+obj, "application/json", strings.NewReader(body))
		if err != nil {
			return 0, err.Error()
		}
		defer r.Body.Close()
		out, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(out)
	}
	for _, w := range [][2]string{
		{"locations", `{"name":"Smoke fridge"}`},
		{"quantity_units", `{"name":"Smoke piece"}`},
	} {
		if code, out := post(w[0], w[1]); code != http.StatusOK {
			return false, w[0] + ": " + out
		}
	}
	var loc, qu string
	for obj, dst := range map[string]*string{"locations": &loc, "quantity_units": &qu} {
		r, err := c.Get(base + "/api/objects/" + obj + "?query[]=name~Smoke")
		if err != nil {
			return false, err.Error()
		}
		var rows []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&rows)
		r.Body.Close()
		if len(rows) == 0 {
			return false, "no " + obj + " row"
		}
		*dst = fmt.Sprint(rows[len(rows)-1]["id"])
	}
	body := fmt.Sprintf(`{"name":"Smoke milk","location_id":%s,"qu_id_purchase":%s,"qu_id_stock":%s,"qu_id_consume":%s,"qu_id_price":%s}`, loc, qu, qu, qu, qu)
	if code, out := post("products", body); code != http.StatusOK {
		return false, "products: " + out
	}
	return true, string(b)
}

func init() {
	// On a read-only rootfs with /var/lib/grocy on a volume: the first user comes from GROCY_HTTP_USER /
	// GROCY_HTTP_PASSWORD, admin/admin does not exist, and after a restart the user signs in, reads
	// /api/system/info and adds a product (the write SQLite 3.54.1 breaks).
	register("grocy", func(t *T) {
		const base = "http://127.0.0.1:18083"
		const pw = "Smoke-Pass-2026"
		u := t.User("1001")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "-v", vol + ":/var/lib/grocy",
				"-e", "GROCY_HTTP_USER=smoke", "-e", "GROCY_HTTP_PASSWORD=" + pw, "-p", "127.0.0.1:18083:8080"})
			// the first GET / runs the database migrations (and creates the first user); it then redirects to /login
			t.WaitHTTP(base+"/", 90, name)
		}
		name := t.Name("")
		start(name)
		if ok, _ := grocyLogin(base, "admin", "admin"); ok {
			t.Fatalf("the default admin/admin account exists")
		}
		if ok, _ := grocyLogin(base, "smoke", "wrong-password-1"); ok {
			t.Fatalf("a wrong password signed in")
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		ok, info := grocyLogin(base, "smoke", pw, true)
		if !ok || !strings.Contains(info, t.Want) {
			t.Fatalf("sign-in or /api/system/info after a restart failed: %q\n%s", info, t.Logs(t.Name("again")))
		}
		t.Logf("smoke test passed (grocy %s: read-only boot, first user from the environment, no admin/admin, wrong password refused, after a restart the session reads /api/system/info and adds a product; user %s)", t.Want, u)
	})
}

// kenerForm posts a SvelteKit form action (with the Origin header SvelteKit's CSRF check wants) and
// returns the status and the Location header.
func kenerForm(base, action string, form url.Values) (int, string) {
	req, _ := http.NewRequest("POST", base+"/account/signin?/"+action, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", base)
	c := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer r.Body.Close()
	return r.StatusCode, r.Header.Get("Location")
}

func init() {
	// On a read-only rootfs with /app/database on a volume and our valkey as its Redis: the migrations run,
	// the first admin signs up through the form, a second sign-up is refused, a wrong password is refused,
	// and the admin signs in after a restart.
	register("kener", func(t *T) {
		const base = "http://127.0.0.1:13000"
		const pw = "Smoke-Pass-2026!"
		u := t.User("1001")
		net := t.Name("net")
		t.Docker("network", "create", net)
		t.Cleanup(func() { docker("network", "rm", net) })
		redis := t.Name("redis")
		t.Cleanup(func() { docker("rm", "-f", redis) })
		t.Docker("run", "-d", "--name", redis, "--network", net, "ghcr.io/quenchworks/images/valkey:9.1.3")
		vol := t.Name("data")
		t.Docker("volume", "create", vol)
		t.Cleanup(func() { docker("volume", "rm", vol) })
		start := func(name string) {
			t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--network", net, "-v", vol + ":/app/database",
				"-e", "REDIS_URL=redis://" + redis + ":6379", "-e", "KENER_SECRET_KEY=quench-smoke-secret-key-0123456789", "-e", "ORIGIN=" + base,
				"-p", "127.0.0.1:13000:3000"})
			t.WaitHTTP(base+"/account/signin", 120, name)
		}
		name := t.Name("")
		start(name)
		signup := url.Values{"name": {"Smoke"}, "email": {"smoke@quench.test"}, "password": {pw}}
		if code, loc := kenerForm(base, "signup", signup); code/100 != 3 || !strings.Contains(loc, "/manage") {
			t.Fatalf("first sign-up: %d %q\n%s", code, loc, t.Logs(name))
		}
		signup.Set("email", "second@quench.test")
		if code, loc := kenerForm(base, "signup", signup); code/100 == 3 {
			t.Fatalf("a second sign-up was accepted (%d %q)", code, loc)
		}
		docker("rm", "-f", name)
		start(t.Name("again"))
		if code, _ := kenerForm(base, "login", url.Values{"email": {"smoke@quench.test"}, "password": {"wrong-password-1"}}); code/100 == 3 {
			t.Fatalf("a wrong password signed in")
		}
		if code, loc := kenerForm(base, "login", url.Values{"email": {"smoke@quench.test"}, "password": {pw}}); code/100 != 3 || !strings.Contains(loc, "/manage") {
			t.Fatalf("sign-in after a restart: %d %q", code, loc)
		}
		t.Logf("smoke test passed (kener %s: read-only boot with valkey, first admin signed up, second sign-up and a wrong password refused, sign-in after a restart; user %s)", t.Want, u)
	})
}
