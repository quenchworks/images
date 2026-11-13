package probe

import (
	"flag"
	"fmt"
	"reflect"
	"strings"
	"time"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

// RethinkSmoke drives a RethinkDB server over the driver protocol: create smoke.t, insert
// {id: 1, name: quench}, read it back, and check the server reports --want as its version.
func RethinkSmoke(args []string) int {
	fs := flag.NewFlagSet("rethinkdb-smoke", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:28015", "driver address")
	want := fs.String("want", "", "expected version prefix (empty: any)")
	if fs.Parse(args) != nil {
		return 2
	}
	s, err := r.Connect(r.ConnectOpts{Address: *addr, Timeout: 10 * time.Second})
	if err != nil {
		return fail("connect: %v", err)
	}
	defer s.Close()
	doc := map[string]any{"id": float64(1), "name": "quench"}
	for _, q := range []r.Term{r.DBCreate("smoke"), r.DB("smoke").TableCreate("t"), r.DB("smoke").Table("t").Insert(doc)} {
		if err := q.Exec(s); err != nil {
			return fail("%v", err)
		}
	}
	var got map[string]any
	if err := r.DB("smoke").Table("t").Get(1).ReadOne(&got, s); err != nil {
		return fail("read: %v", err)
	}
	if !reflect.DeepEqual(got, doc) {
		return fail("read back %v, want %v", got, doc)
	}
	var ver string
	if err := r.DB("rethinkdb").Table("server_status").Nth(0).Field("process").Field("version").ReadOne(&ver, s); err != nil {
		return fail("version: %v", err)
	}
	fmt.Println("document:", got)
	fmt.Println("version:", ver)
	if *want != "" && !strings.HasPrefix(ver, "rethinkdb "+*want) {
		return fail("version %q, want rethinkdb %s", ver, *want)
	}
	return 0
}
