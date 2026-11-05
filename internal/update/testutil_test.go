package update

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/TechSavage1989/weightmaxer/internal/version"
)

// probeExe cross-compiles a tiny Windows GUI program carrying the Weightmaxer
// identity marker for ver, so archive and PE checks run against a real PE file.
var probeMu sync.Mutex
var probeCache = map[string][]byte{}

func probeExe(t *testing.T, ver string) []byte {
	t.Helper()
	probeMu.Lock()
	defer probeMu.Unlock()
	if b, ok := probeCache[ver]; ok {
		return b
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	os.WriteFile(src, []byte("package main\nvar id = \"x\"\nfunc main() { println(id) }\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module probe\ngo 1.21\n"), 0o600)
	out := filepath.Join(dir, "probe.exe")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-H windowsgui -X main.id="+version.IdentityFor(ver), "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building probe: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	probeCache[ver] = b
	return b
}

func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	zw.Close()
	return buf.Bytes()
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// feed is a fake GitHub API plus release downloads over HTTPS.
type feed struct {
	srv      *httptest.Server
	mu       sync.Mutex
	status   int
	headers  map[string]string
	etag     string
	release  map[string]any
	files    map[string][]byte
	requests int
	sawINM   []string
}

func newFeed(t *testing.T) *feed {
	f := &feed{status: 200, files: map[string][]byte{}, headers: map[string]string{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/repos/TechSavage1989/weightmaxer/releases/latest" {
			f.requests++
			f.sawINM = append(f.sawINM, r.Header.Get("If-None-Match"))
			for k, v := range f.headers {
				w.Header().Set(k, v)
			}
			if f.status != 200 {
				w.WriteHeader(f.status)
				return
			}
			if f.etag != "" && r.Header.Get("If-None-Match") == f.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			if f.etag != "" {
				w.Header().Set("ETag", f.etag)
			}
			fmt.Fprint(w, mustJSON(f.release))
			return
		}
		if b, ok := f.files[r.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *feed) client(t *testing.T, current string) *Client {
	pool := x509.NewCertPool()
	pool.AddCert(f.srv.Certificate())
	u, _ := url.Parse(f.srv.URL)
	return &Client{
		Config: Config{
			APIBase: f.srv.URL, Owner: "TechSavage1989", Repo: "weightmaxer",
			DownloadPrefix: f.srv.URL + "/download/", ReleasesPage: f.srv.URL + "/releases/",
			AllowedHosts: map[string]bool{u.Hostname(): true}, UserAgent: "test",
			TLS: &tls.Config{RootCAs: pool},
		},
		Current:   current,
		CachePath: filepath.Join(t.TempDir(), "cache.json"),
	}
}

// publish sets up release ver with a zip containing exe.
func (f *feed) publish(ver string, zipData []byte, digest bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	zp := "/download/v" + ver + "/" + AssetName
	sp := "/download/v" + ver + "/" + SumsName
	sums := []byte(sha(zipData) + "  " + AssetName + "\n")
	f.files[zp], f.files[sp] = zipData, sums
	za := map[string]any{"name": AssetName, "browser_download_url": f.srv.URL + zp, "size": len(zipData)}
	if digest {
		za["digest"] = "sha256:" + sha(zipData)
	}
	f.release = map[string]any{
		"tag_name": "v" + ver, "name": "Weightmaxer " + ver, "html_url": f.srv.URL + "/releases/tag/v" + ver,
		"body": "## What's new\n- Faster charts\n- Fixed the log", "assets": []any{za,
			map[string]any{"name": SumsName, "browser_download_url": f.srv.URL + sp, "size": len(sums)}},
	}
}
