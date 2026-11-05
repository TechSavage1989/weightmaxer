//go:build windows

// Package e2e runs the real update path on Windows: an installed update-test
// build of an older version checks a local HTTPS release feed, downloads and
// verifies the new version, hands over to the helper, which swaps the
// executable, starts the new version and waits for its window. Run with:
//
//	go test ./tests/e2e -v -timeout 20m
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TechSavage1989/weightmaxer/internal/update"
	"github.com/TechSavage1989/weightmaxer/internal/version"
)

const oldV, newV = "0.9.0", "0.9.1"

var root, _ = filepath.Abs("../..")

func build(t *testing.T, v string, pkg bool) string {
	t.Helper()
	out := filepath.Join(root, "build", "e2e", v)
	args := []string{filepath.Join(root, "scripts", "build.py"), "--version", v, "--tags", "updatetest", "--out", out}
	if !pkg {
		args = append(args, "--no-package")
	}
	cmd := exec.Command(python(), args...)
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", v, err, b)
	}
	return out
}

func python() string {
	if p, err := exec.LookPath("python"); err == nil {
		return p
	}
	return "python3"
}

type server struct {
	srv  *httptest.Server
	zip  []byte
	sums []byte
}

func serve(t *testing.T, zip []byte, sums []byte) (*server, string) {
	s := &server{zip: zip, sums: sums}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := s.srv.URL
		switch r.URL.Path {
		case "/repos/TechSavage1989/weightmaxer/releases/latest":
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v" + newV, "name": "Weightmaxer " + newV, "html_url": base + "/releases/tag/v" + newV,
				"body": "## What's new\n- End-to-end test release",
				"assets": []any{
					map[string]any{"name": update.AssetName, "browser_download_url": base + "/download/v" + newV + "/" + update.AssetName, "size": len(s.zip)},
					map[string]any{"name": update.SumsName, "browser_download_url": base + "/download/v" + newV + "/" + update.SumsName, "size": len(s.sums)},
				},
			})
		case "/download/v" + newV + "/" + update.AssetName:
			w.Write(s.zip)
		case "/download/v" + newV + "/" + update.SumsName:
			w.Write(s.sums)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.srv.Certificate().Raw}), 0o600)
	return s, ca
}

type install struct {
	home, target, data string
}

// installOld copies the old build into a fresh folder under a custom file name
// (to prove the updater replaces whatever the user named it) with saved data.
func installOld(t *testing.T, oldDir, name string) install {
	dir := t.TempDir()
	in := install{home: filepath.Join(dir, "home"), target: filepath.Join(dir, "app", name)}
	os.MkdirAll(filepath.Dir(in.target), 0o700)
	b, _ := os.ReadFile(filepath.Join(oldDir, "Weightmaxer.exe"))
	os.WriteFile(in.target, b, 0o700)
	in.data = filepath.Join(in.home, "Roaming", "Weightmaxer", "data.json")
	os.MkdirAll(filepath.Dir(in.data), 0o700)
	os.WriteFile(in.data, []byte(`{"schema":1,"units":"met","log":[{"d":"2026-11-01","kg":88.5}],"profile":{"w":"90"}}`), 0o600)
	return in
}

func (in install) local(p ...string) string {
	return filepath.Join(append([]string{in.home, "Local", "Weightmaxer"}, p...)...)
}

func (in install) start(t *testing.T, api, ca string, extra ...string) *exec.Cmd {
	cmd := exec.Command(in.target)
	cmd.Dir = filepath.Dir(in.target)
	cmd.Env = append(os.Environ(),
		"WEIGHTMAXER_TEST_HOME="+in.home, "WEIGHTMAXER_TEST_API="+api, "WEIGHTMAXER_TEST_CA="+ca,
		"WEIGHTMAXER_TEST_AUTO_UPDATE=1", "WEIGHTMAXER_TEST_HEALTH_TIMEOUT=45s")
	cmd.Env = append(cmd.Env, extra...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(killAll)
	return cmd
}

func killAll() {
	exec.Command("taskkill", "/F", "/T", "/FI", "IMAGENAME eq Weightmaxer*").Run()
	time.Sleep(time.Second)
}

func waitFor(t *testing.T, d time.Duration, what string, cond func() bool, in install) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	logs, _ := os.ReadFile(in.local("logs", "weightmaxer.log"))
	t.Fatalf("timed out waiting for %s\napp log:\n%s", what, logs)
}

func hasIdentity(path, v string) bool {
	b, err := os.ReadFile(path)
	return err == nil && bytes.Contains(b, []byte(version.IdentityFor(v)))
}

func appLog(in install) string {
	b, _ := os.ReadFile(in.local("logs", "weightmaxer.log"))
	return string(b)
}

func TestUpdate(t *testing.T) {
	killAll()
	oldDir := build(t, oldV, false)
	newDir := build(t, newV, true)
	zip, err := os.ReadFile(filepath.Join(newDir, update.AssetName))
	if err != nil {
		t.Fatal(err)
	}
	sums, _ := os.ReadFile(filepath.Join(newDir, update.SumsName))

	t.Run("installs, relaunches, keeps data and one backup, cleans up", func(t *testing.T) {
		s, ca := serve(t, zip, sums)
		in := installOld(t, oldDir, "Weightmaxer (2).exe")
		before, _ := os.ReadFile(in.data)
		in.start(t, s.srv.URL, ca)
		var out update.Outcome
		waitFor(t, 3*time.Minute, "update result", func() bool { return update.ReadJSON(in.local("updates", "result.json"), &out) == nil }, in)
		if !out.OK || out.From != oldV || out.To != newV {
			t.Fatalf("outcome = %+v\n%s", out, appLog(in))
		}
		if !hasIdentity(in.target, newV) {
			t.Fatal("target is not the new version")
		}
		var h update.Health
		if update.ReadJSON(in.local("updates", "health.json"), &h) != nil && !strings.Contains(appLog(in), "window ready (visible=true)") {
			t.Fatal("new version never reported a visible window")
		}
		waitFor(t, 2*time.Minute, "cleanup after the helper exits", func() bool {
			ents, _ := os.ReadDir(in.local("updates"))
			for _, e := range ents {
				if e.Name() != "result.json" {
					return false
				}
			}
			return strings.Contains(appLog(in), "update files cleaned up")
		}, in)
		backups, _ := os.ReadDir(in.local("backup"))
		if len(backups) != 1 || backups[0].Name() != "Weightmaxer-"+oldV+".exe" || !hasIdentity(in.local("backup", backups[0].Name()), oldV) {
			t.Fatalf("backups = %v", backups)
		}
		if after, _ := os.ReadFile(in.data); !bytes.Equal(before, after) {
			t.Fatalf("data changed:\n%s\n%s", before, after)
		}
		for _, leftover := range []string{in.target + ".new", in.target + ".new.tmp"} {
			if _, err := os.Stat(leftover); err == nil {
				t.Fatalf("%s left behind", leftover)
			}
		}
		if ents, _ := os.ReadDir(filepath.Dir(in.target)); len(ents) != 1 {
			t.Fatalf("app folder has extra files: %v", ents)
		}
		t.Logf("app log:\n%s", appLog(in))
	})
	killAll()

	t.Run("rejects a download whose checksum doesn't match", func(t *testing.T) {
		badSum := sha256.Sum256([]byte("something else"))
		s, ca := serve(t, zip, []byte(hex.EncodeToString(badSum[:])+"  "+update.AssetName+"\n"))
		in := installOld(t, oldDir, "Weightmaxer.exe")
		in.start(t, s.srv.URL, ca)
		waitFor(t, 2*time.Minute, "download rejection", func() bool { return strings.Contains(appLog(in), "download failed") }, in)
		if !strings.Contains(appLog(in), "checksum") {
			t.Fatalf("unexpected failure:\n%s", appLog(in))
		}
		if !hasIdentity(in.target, oldV) {
			t.Fatal("target changed after a rejected download")
		}
		if _, err := os.Stat(in.local("updates", "result.json")); err == nil {
			t.Fatal("an install was attempted")
		}
	})
	killAll()

	t.Run("rolls back when the new version doesn't open", func(t *testing.T) {
		s, ca := serve(t, zip, sums)
		in := installOld(t, oldDir, "Weightmaxer.exe")
		before, _ := os.ReadFile(in.data)
		in.start(t, s.srv.URL, ca, "WEIGHTMAXER_TEST_FAIL_HEALTH=1")
		var out update.Outcome
		waitFor(t, 4*time.Minute, "rollback result", func() bool {
			return update.ReadJSON(in.local("updates", "result.json"), &out) == nil && out.RolledBack
		}, in)
		if out.OK || !strings.Contains(out.Error, "didn't open its window") {
			t.Fatalf("outcome = %+v", out)
		}
		if !hasIdentity(in.target, oldV) {
			t.Fatal("old version was not restored")
		}
		waitFor(t, time.Minute, "old version relaunched", func() bool { return strings.Contains(appLog(in), "--update-failed=") }, in)
		if after, _ := os.ReadFile(in.data); !bytes.Equal(before, after) {
			t.Fatal("data changed during rollback")
		}
		fmt.Println(appLog(in))
	})
	killAll()
}
