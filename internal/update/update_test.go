package update

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestCheckFindsNewerReleaseAndUsesETag(t *testing.T) {
	f := newFeed(t)
	f.etag = `"abc"`
	f.publish("1.1.0", []byte("zip"), true)
	c := f.client(t, "1.0.0")
	r := c.Check(context.Background())
	if r.Kind != KindAvailable || r.Release.Version != "1.1.0" || r.NotModified {
		t.Fatalf("first check = %+v", r)
	}
	r = c.Check(context.Background())
	if r.Kind != KindAvailable || !r.NotModified {
		t.Fatalf("second check should be a cached 304: %+v", r)
	}
	if f.sawINM[0] != "" || f.sawINM[1] != `"abc"` {
		t.Fatalf("If-None-Match headers = %q", f.sawINM)
	}
}

func TestCheckUpToDateAndNoRelease(t *testing.T) {
	f := newFeed(t)
	f.publish("1.0.0", []byte("zip"), false)
	if r := f.client(t, "1.0.0").Check(context.Background()); r.Kind != KindUpToDate {
		t.Fatalf("same version = %+v", r)
	}
	if r := f.client(t, "1.2.0").Check(context.Background()); r.Kind != KindUpToDate {
		t.Fatalf("newer local = %+v", r)
	}
	f.status = 404
	if r := f.client(t, "1.0.0").Check(context.Background()); r.Kind != KindNoRelease {
		t.Fatalf("404 = %+v", r)
	}
}

func TestFailuresNeverSayUpToDate(t *testing.T) {
	f := newFeed(t)
	f.publish("1.0.0", []byte("zip"), false)
	c := f.client(t, "1.0.0")
	f.status = 500
	if r := c.Check(context.Background()); r.Kind != KindError || !r.Kind.Failed() {
		t.Fatalf("500 = %+v", r)
	}
	f.srv.Close()
	r := c.Check(context.Background())
	if r.Kind != KindOffline || !r.Kind.Failed() || strings.Contains(strings.ToLower(r.Message), "up to date") {
		t.Fatalf("offline = %+v", r)
	}
	dev := &Client{Current: "0.0.0-dev"}
	if r := dev.Check(context.Background()); r.Kind != KindError {
		t.Fatalf("dev build = %+v", r)
	}
}

func TestRateLimitsPauseChecks(t *testing.T) {
	f := newFeed(t)
	f.publish("1.0.0", []byte("zip"), false)
	c := f.client(t, "1.0.0")
	now := time.Now().Truncate(time.Second)
	c.Now = func() time.Time { return now }

	f.status = 403
	f.headers = map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": itoa(now.Add(20 * time.Minute).Unix())}
	r := c.Check(context.Background())
	if r.Kind != KindRateLimited || r.PauseUntil.Sub(now) != 20*time.Minute {
		t.Fatalf("403 limit = %+v", r)
	}
	f.status = 429
	f.headers = map[string]string{"Retry-After": "90"}
	r = c.Check(context.Background())
	if r.Kind != KindRateLimited || r.PauseUntil.Sub(now) != 90*time.Second {
		t.Fatalf("429 retry-after = %+v", r)
	}
	// A successful answer with few requests left still pauses automatic checks.
	f.status = 200
	f.headers = map[string]string{"X-RateLimit-Remaining": "3", "X-RateLimit-Reset": itoa(now.Add(10 * time.Minute).Unix())}
	r = c.Check(context.Background())
	if r.Kind != KindUpToDate || r.PauseUntil.Sub(now) != 10*time.Minute {
		t.Fatalf("low remaining = %+v", r)
	}
	// Forbidden without rate-limit headers is an error, not a pause.
	f.status = 403
	f.headers = map[string]string{}
	if r = c.Check(context.Background()); r.Kind != KindError || !r.PauseUntil.IsZero() {
		t.Fatalf("plain 403 = %+v", r)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestCheckRejectsPrereleaseAndMissingAssets(t *testing.T) {
	f := newFeed(t)
	f.publish("1.1.0", []byte("zip"), false)
	f.release["prerelease"] = true
	if r := f.client(t, "1.0.0").Check(context.Background()); r.Kind != KindError {
		t.Fatalf("prerelease = %+v", r)
	}
	f.release["prerelease"] = false
	f.release["assets"] = []any{}
	if r := f.client(t, "1.0.0").Check(context.Background()); r.Kind != KindError {
		t.Fatalf("no assets = %+v", r)
	}
}

func TestWhatsNew(t *testing.T) {
	b, more := WhatsNew("## What's new\n- **Charts** load faster\n- Fixed [log](http://x) dates\n")
	if len(b) != 2 || b[0] != "Charts load faster" || b[1] != "Fixed log dates" || more {
		t.Fatalf("simple notes = %q more=%v", b, more)
	}
	_, more = WhatsNew("- a\n- b\n- c\n- d\n- e")
	if !more {
		t.Fatal("five bullets should offer Read more")
	}
	_, more = WhatsNew("- a\n\nA longer paragraph explaining things.")
	if !more {
		t.Fatal("extra paragraph should offer Read more")
	}
	b, more = WhatsNew("- " + strings.Repeat("x", 300))
	if !more || len([]rune(b[0])) != 140 {
		t.Fatal("long bullet should be shortened")
	}
}

func TestDownloadVerifyAndExtract(t *testing.T) {
	exe := probeExe(t, "1.1.0")
	good := makeZip(t, map[string][]byte{ExeName: exe, "README.txt": []byte("hi")})
	f := newFeed(t)
	f.publish("1.1.0", good, true)
	c := f.client(t, "1.0.0")
	r := c.Check(context.Background())
	dir := t.TempDir()
	var calls int32
	zp, err := c.Download(context.Background(), r.Release, dir, func(done, total int64) { atomic.AddInt32(&calls, 1) })
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("no progress reported")
	}
	out, err := ExtractExe(zp, "1.1.0", filepath.Join(dir, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); len(b) != len(exe) {
		t.Fatal("extracted exe differs")
	}
	// Wrong version marker is rejected.
	if _, err := ExtractExe(zp, "1.2.0", filepath.Join(dir, "s2")); err == nil || !strings.Contains(err.Error(), "isn't Weightmaxer 1.2.0") {
		t.Fatalf("version mismatch err = %v", err)
	}
}

func TestDownloadRejectsTampering(t *testing.T) {
	exe := probeExe(t, "1.1.0")
	good := makeZip(t, map[string][]byte{ExeName: exe})
	cases := map[string]func(f *feed){
		"hash mismatch": func(f *feed) {
			bad := append([]byte{}, good...)
			bad[len(bad)/2] ^= 0xff
			f.files["/download/v1.1.0/"+AssetName] = bad
		},
		"digest mismatch": func(f *feed) {
			f.release["assets"].([]any)[0].(map[string]any)["digest"] = "sha256:" + strings.Repeat("0", 64)
		},
		"foreign host": func(f *feed) {
			f.release["assets"].([]any)[0].(map[string]any)["browser_download_url"] = "https://evil.example.com/download/v1.1.0/" + AssetName
		},
		"plain http": func(f *feed) {
			u := f.release["assets"].([]any)[0].(map[string]any)["browser_download_url"].(string)
			f.release["assets"].([]any)[0].(map[string]any)["browser_download_url"] = strings.Replace(u, "https:", "http:", 1)
		},
		"wrong size": func(f *feed) {
			f.release["assets"].([]any)[0].(map[string]any)["size"] = len(good) + 10
		},
		"no checksum entry": func(f *feed) {
			f.files["/download/v1.1.0/"+SumsName] = []byte(sha(good) + "  other.zip\n")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFeed(t)
			f.publish("1.1.0", good, true)
			mutate(f)
			c := f.client(t, "1.0.0")
			r := c.Check(context.Background())
			if r.Release == nil {
				t.Fatalf("check = %+v", r)
			}
			dir := t.TempDir()
			if _, err := c.Download(context.Background(), r.Release, dir, nil); err == nil {
				t.Fatal("tampered download accepted")
			}
			left, _ := os.ReadDir(dir)
			if len(left) != 0 {
				t.Fatalf("left files behind: %v", left)
			}
		})
	}
}

func TestExtractRejectsUnsafeArchives(t *testing.T) {
	exe := probeExe(t, "1.1.0")
	cases := map[string][]byte{
		"traversal":        makeZip(t, map[string][]byte{"../Weightmaxer.exe": exe}),
		"subfolder":        makeZip(t, map[string][]byte{"bin/Weightmaxer.exe": exe}),
		"backslash":        makeZip(t, map[string][]byte{`..\Weightmaxer.exe`: exe}),
		"absolute":         makeZip(t, map[string][]byte{"/Weightmaxer.exe": exe}),
		"drive":            makeZip(t, map[string][]byte{"C:Weightmaxer.exe": exe}),
		"extra executable": makeZip(t, map[string][]byte{ExeName: exe, "helper.exe": exe}),
		"missing exe":      makeZip(t, map[string][]byte{"README.txt": []byte("x")}),
		"bomb":             makeZip(t, map[string][]byte{ExeName: make([]byte, 30<<20)}),
		"not a program":    makeZip(t, map[string][]byte{ExeName: []byte(strings.Repeat("MZ not really", 100))}),
		"case duplicate":   makeZip(t, map[string][]byte{ExeName: exe, "weightmaxer.exe": exe}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "u.zip")
			os.WriteFile(p, data, 0o600)
			if _, err := ExtractExe(p, "1.1.0", filepath.Join(t.TempDir(), "s")); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
	// Not a zip at all.
	p := filepath.Join(t.TempDir(), "u.zip")
	os.WriteFile(p, []byte("garbage"), 0o600)
	if _, err := ExtractExe(p, "1.1.0", t.TempDir()); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestCleanupKeepsResultAndBackup(t *testing.T) {
	root := t.TempDir()
	up := filepath.Join(root, "updates")
	os.MkdirAll(filepath.Join(up, "staging-1.1.0"), 0o700)
	for _, n := range []string{"result.json", "pending.json", "health.json", "Weightmaxer-1.1.0-win-x64.zip", "x.zip.part", "keep-me.txt"} {
		os.WriteFile(filepath.Join(up, n), []byte("x"), 0o600)
	}
	target := filepath.Join(root, "Weightmaxer.exe")
	os.WriteFile(target+".new", []byte("x"), 0o600)
	if err := Cleanup(up, target); err != nil {
		t.Fatal(err)
	}
	left, _ := os.ReadDir(up)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "keep-me.txt,result.json" {
		t.Fatalf("left = %v", names)
	}
	if _, err := os.Stat(target + ".new"); err == nil {
		t.Fatal(".new left behind")
	}
}

func TestKeepOneBackup(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "My Weightmaxer.exe")
	os.WriteFile(target, []byte("v2"), 0o700)
	bdir := filepath.Join(root, "backup")
	os.MkdirAll(bdir, 0o700)
	os.WriteFile(filepath.Join(bdir, "Weightmaxer-1.0.0.exe"), []byte("v1"), 0o700)
	p, err := keepOneBackup(target, bdir, "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	left, _ := os.ReadDir(bdir)
	if len(left) != 1 || filepath.Base(p) != "Weightmaxer-1.1.0.exe" {
		t.Fatalf("backups = %v", left)
	}
}
