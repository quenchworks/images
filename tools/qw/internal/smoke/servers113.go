package smoke

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// hs256 signs claims as a compact HS256 access token (RFC 9068 wants typ at+jwt).
func hs256(key string, claims map[string]any) string {
	enc := base64.RawURLEncoding
	h := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"at+jwt"}`))
	c, _ := json.Marshal(claims)
	unsigned := h + "." + enc.EncodeToString(c)
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(unsigned))
	return unsigned + "." + enc.EncodeToString(m.Sum(nil))
}

func init() {
	// On a read-only rootfs: the hub serves its welcome page, an anonymous subscriber receives an update
	// that a publisher posts with an RFC 9068 token (iss, aud, exp, authorization_details), and a
	// publish without a token is refused with 401.
	register("mercure", func(t *T) {
		const base = "http://127.0.0.1:18085"
		const key = "smoke-publisher-key-0123456789abcdef"
		const issuer = "https://issuer.quench.test"
		const topic = "https://quench.test/smoke"
		u := t.User("1001")
		t.Contains(t.RunAny("version"), "v"+t.Want, "mercure version")
		name := t.Name("")
		t.Start(name, []string{"--read-only", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/data:rw,uid=1001,gid=1001", "--tmpfs", "/config:rw,uid=1001,gid=1001",
			"-p", "127.0.0.1:18085:8080", "-e", "MERCURE_PUBLISHER_JWT_KEY=" + key, "-e", "MERCURE_SUBSCRIBER_JWT_KEY=" + key,
			"-e", "MERCURE_TRUSTED_ISSUERS=" + issuer, "-e", "MERCURE_EXTRA_DIRECTIVES=anonymous"})
		t.waitFor(60, time.Second, name, "mercure never served its welcome page", func() bool {
			if t.Docker("inspect", "-f", "{{.State.Running}}", name) != "true" {
				t.Fatalf("mercure exited\n%s", t.Logs(name))
			}
			c, b := Get(base + "/")
			return c == 200 && strings.Contains(b, "Welcome to Mercure")
		})
		got := make(chan string, 1)
		go func() {
			resp, err := http.Get(base + "/.well-known/mercure?match=" + url.QueryEscape(topic))
			if err != nil {
				got <- "subscribe: " + err.Error()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				got <- "subscribe answered " + resp.Status
				return
			}
			sc := bufio.NewScanner(resp.Body)
			for sc.Scan() {
				if strings.HasPrefix(sc.Text(), "data: ") {
					got <- strings.TrimPrefix(sc.Text(), "data: ")
					return
				}
			}
			got <- "subscription ended"
		}()
		time.Sleep(2 * time.Second) // let the subscription register before publishing
		form := url.Values{"topic": {topic}, "data": {"quench mercure smoke"}}.Encode()
		hdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
		if c, _ := DoH("POST", base+"/.well-known/mercure", form, hdr); c != 401 {
			t.Fatalf("a publish without a token answered %d, expected 401", c)
		}
		tok := hs256(key, map[string]any{"iss": issuer, "aud": base + "/.well-known/mercure", "exp": time.Now().Add(time.Hour).Unix(),
			"authorization_details": []map[string]any{{"type": "https://mercure.rocks/authorization-detail", "actions": []string{"publish"}, "topics": []map[string]string{{"match": topic}}}}})
		hdr["Authorization"] = "Bearer " + tok
		if c, b := DoH("POST", base+"/.well-known/mercure", form, hdr); c != 200 {
			t.Fatalf("publish with a token answered %d: %s\n%s", c, b, t.Logs(name))
		}
		select {
		case d := <-got:
			if d != "quench mercure smoke" {
				t.Fatalf("the subscriber got %q", d)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("the subscriber never received the update\n%s", t.Logs(name))
		}
		t.Logf("smoke test passed (mercure %s: read-only boot, welcome page, token publish delivered to an SSE subscriber, unauthenticated publish refused; user %s)", t.Want, u)
	})
}
