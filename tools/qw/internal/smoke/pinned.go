package smoke

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"time"
)

// pinnedClient is an HTTPS client that trusts only the PEM certificates in caFile (a CA or a self-signed
// certificate the test generated) and checks the server name against serverName.
func pinnedClient(caFile, serverName string) (*http.Client, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem)
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{ForceAttemptHTTP2: true,
		TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: serverName}}}, nil
}
