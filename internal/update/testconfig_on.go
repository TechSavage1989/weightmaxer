//go:build updatetest

package update

import (
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"os"
	"strings"
	"time"
)

// Update test builds read a local HTTPS feed from the environment so the
// Windows end-to-end test can serve releases without touching GitHub.
func applyTestOverrides(c *Config) {
	api := os.Getenv("WEIGHTMAXER_TEST_API")
	if api == "" {
		return
	}
	u, err := url.Parse(api)
	if err != nil {
		return
	}
	c.APIBase = strings.TrimSuffix(api, "/")
	c.DownloadPrefix = c.APIBase + "/download/"
	c.ReleasesPage = c.APIBase + "/releases/"
	c.AllowedHosts = map[string]bool{strings.ToLower(u.Hostname()): true}
	if pemPath := os.Getenv("WEIGHTMAXER_TEST_CA"); pemPath != "" {
		if pem, err := os.ReadFile(pemPath); err == nil {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(pem)
			c.TLS = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		}
	}
}

// TestAutoUpdate makes an update test build check, download and install on its
// own so the end-to-end test needs no clicks. Release builds never do this.
func TestAutoUpdate() bool { return os.Getenv("WEIGHTMAXER_TEST_AUTO_UPDATE") == "1" }

// TestFailHealth makes a test build never report a healthy window, to prove rollback.
func TestFailHealth() bool { return os.Getenv("WEIGHTMAXER_TEST_FAIL_HEALTH") == "1" }

func init() {
	if s := os.Getenv("WEIGHTMAXER_TEST_HEALTH_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			HealthTimeout = d
		}
	}
}
