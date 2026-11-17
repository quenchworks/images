package bump

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A host that labels a .tar.gz "Content-Encoding: gzip" must be hashed as the bytes it sends.
func TestFetchShaKeepsGzipBytes(t *testing.T) {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write([]byte("tarball body"))
	w.Close()
	body := gz.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/gzip")
		rw.Header().Set("Content-Encoding", "gzip")
		rw.Write(body)
	}))
	defer srv.Close()
	got, err := fetchSha(srv.URL+"/x.tar.gz", "sha256")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if want := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("hashed the decompressed body: got %s, want %s", got, want)
	}
}
