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
