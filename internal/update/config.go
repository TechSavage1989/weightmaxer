// Package update checks GitHub releases for a newer Weightmaxer, downloads and
// verifies it, and installs it through a helper process with rollback. It never
// downloads or installs on its own: the app calls Download and Install only
// after the user presses the matching button.
package update

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// AssetName is the Windows download attached to every release.
	AssetName = "Weightmaxer-win-x64.zip"
	// SumsName lists the SHA-256 of every release download.
	SumsName = "SHA256SUMS.txt"
	// ExeName is the stable launch filename inside AssetName.
	ExeName = "Weightmaxer.exe"

	maxZipSize  = 64 << 20
	maxSumsSize = 64 << 10
	maxAPISize  = 1 << 20
)

// Config says where releases come from and which hosts may serve them.
type Config struct {
	APIBase        string // https://api.github.com
	Owner, Repo    string
	DownloadPrefix string // every asset URL must start with this + "v<version>/"
	ReleasesPage   string // "Read more" links must start with this
	AllowedHosts   map[string]bool
	UserAgent      string
	TLS            *tls.Config
}

// Production is the public GitHub release feed.
func Production(currentVersion string) Config {
	c := Config{
		APIBase:        "https://api.github.com",
		Owner:          "TechSavage1989",
		Repo:           "weightmaxer",
		DownloadPrefix: "https://github.com/TechSavage1989/weightmaxer/releases/download/",
		ReleasesPage:   "https://github.com/TechSavage1989/weightmaxer/releases/",
		AllowedHosts: map[string]bool{
			"api.github.com":                        true,
			"github.com":                            true,
			"objects.githubusercontent.com":         true,
			"release-assets.githubusercontent.com":  true,
			"github-releases.githubusercontent.com": true,
		},
		UserAgent: "Weightmaxer/" + currentVersion + " (update check)",
	}
	applyTestOverrides(&c)
	return c
}

func (c Config) checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing non-HTTPS address %s", u.Redacted())
	}
	if u.User != nil || !c.AllowedHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("refusing download from untrusted host %q", u.Hostname())
	}
	return nil
}

func (c Config) client(timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if c.TLS != nil {
		tr.TLSClientConfig = c.TLS
	}
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return c.checkURL(req.URL.String())
		},
	}
}
