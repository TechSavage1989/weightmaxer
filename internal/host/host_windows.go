//go:build windows

// Package host runs the Weightmaxer window: a WebView2 control showing the
// embedded web page, bindings for saving data, and the update flow.
package host

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/TechSavage1989/weightmaxer/internal/paths"
	"github.com/TechSavage1989/weightmaxer/internal/store"
	"github.com/TechSavage1989/weightmaxer/internal/update"
	"github.com/TechSavage1989/weightmaxer/internal/version"
	"github.com/TechSavage1989/weightmaxer/web"
)

// Options come from the command line.
type Options struct {
	PostUpdate   string // pending.json of an update that just installed this copy
	UpdateFailed string // result.json of an update that was rolled back
	SelfTest     string // write a health report here once the window is up, then exit
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procGetDpiForSystem     = user32.NewProc("GetDpiForSystem")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
)

const windowTitle = "Weightmaxer"

// notice is a one-off message shown in the banner (update finished or rolled back).
type notice struct {
	OK     bool   `json:"ok"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

type checkInfo struct {
	Kind        update.Kind `json:"kind,omitempty"`
	Message     string      `json:"message,omitempty"`
	At          *time.Time  `json:"at,omitempty"`
	LastSuccess *time.Time  `json:"lastSuccess,omitempty"`
	Checking    bool        `json:"checking"`
}

type progress struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
}

// uiState is pushed to the page as JSON on every change.
type uiState struct {
	Current  string    `json:"current"`
	Latest   string    `json:"latest,omitempty"`
	Dev      bool      `json:"dev"`
	Phase    string    `json:"phase"` // idle, available, downloading, ready, installing, error
	Check    checkInfo `json:"check"`
	Notes    []string  `json:"notes,omitempty"`
	More     bool      `json:"more"`
	Progress progress  `json:"progress"`
	Error    string    `json:"error,omitempty"`
	Notice   *notice   `json:"notice,omitempty"`
}

type app struct {
	opts   Options
	dirs   paths.Dirs
	store  *store.Store
	client *update.Client
	sched  *update.Scheduler
	w      webview2.WebView
	hwnd   uintptr
	ctx    context.Context

	mu      sync.Mutex
	st      uiState
	release *update.Release
	staged  string
	ready   bool
}

// Run shows the window and returns the process exit code.
func Run(opts Options) int {
	runtime.LockOSThread()
	dirs, err := paths.Default()
	if err == nil {
		err = dirs.Ensure()
	}
	if err != nil {
		messageBox("Weightmaxer can't create its settings folder: " + err.Error())
		return 1
	}
	closeLog := openLog(dirs.LogFile())
	defer closeLog()
	log.Printf("Weightmaxer %s starting [%s] (%s)", version.Version, version.Identity, strings.Join(os.Args[1:], " "))

	if !singleInstance() {
		log.Print("another copy is running; bringing it to the front")
		return 0
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{opts: opts, dirs: dirs, store: store.New(dirs.DataFile()), ctx: ctx}
	a.st = uiState{Current: version.Version, Dev: version.IsDev(), Phase: "idle"}
	a.client = &update.Client{Config: update.Production(version.Version), Current: version.Version, CachePath: dirs.UpdateCache()}

	scale := func(px uint) uint { return px * dpi() / 96 }
	a.w = webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  dirs.WebViewData(),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: windowTitle, Width: scale(1220), Height: scale(860), IconId: 1, Center: true,
		},
	})
	if a.w == nil {
		messageBox("Weightmaxer needs the Microsoft Edge WebView2 Runtime, which is part of Windows 10 and 11 but is missing or damaged on this PC.\n\nInstall it from https://developer.microsoft.com/microsoft-edge/webview2/ and start Weightmaxer again.")
		return 1
	}
	a.hwnd = uintptr(a.w.Window())
	a.w.SetSize(int(scale(780)), int(scale(560)), webview2.HintMin)
	a.bind()
	a.w.SetHtml(web.Page())

	a.loadNotices()
	if opts.PostUpdate == "" && opts.UpdateFailed == "" {
		// Leftovers from an earlier update (for example after a power cut).
		go func() { _ = update.Cleanup(dirs.Updates(), exePath()) }()
	}
	if !version.IsDev() && !update.TestAutoUpdate() {
		a.sched = &update.Scheduler{
			Policy:   update.DefaultPolicy,
			Check:    a.client.Check,
			Active:   a.windowActive,
			OnStart:  a.onCheckStart,
			OnResult: a.onCheckResult,
		}
		go a.sched.Run(ctx)
		go a.watchFocus(ctx)
	}
	a.w.Run()
	cancel()
	log.Print("window closed")
	return 0
}

func (a *app) bind() {
	must := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}
	must(a.w.Bind("wmxLoad", func() (string, error) { return a.store.Load() }))
	must(a.w.Bind("wmxSave", func(doc string) error {
		err := a.store.Save(doc)
		if err != nil {
			log.Printf("save failed: %v", err)
		}
		return err
	}))
	must(a.w.Bind("wmxInfo", func() map[string]any {
		return map[string]any{"version": version.Version, "dataFolder": a.dirs.Roaming, "dev": version.IsDev()}
	}))
	must(a.w.Bind("wmxReady", a.onReady))
	must(a.w.Bind("wmxCheckNow", func() map[string]any {
		if a.sched == nil {
			return map[string]any{"accepted": false, "message": "Updates are off in development builds."}
		}
		ok, msg := a.sched.Manual()
		return map[string]any{"accepted": ok, "message": msg}
	}))
	must(a.w.Bind("wmxDownload", func() { go a.download() }))
	must(a.w.Bind("wmxInstall", func() string {
		if err := a.install(); err != nil {
			return err.Error()
		}
		return ""
	}))
	must(a.w.Bind("wmxOpenNotes", func() {
		a.mu.Lock()
		rel := a.release
		a.mu.Unlock()
		if rel != nil && strings.HasPrefix(rel.HTMLURL, a.client.ReleasesPage) {
			_ = windows.ShellExecute(0, ptr("open"), ptr(rel.HTMLURL), nil, nil, windows.SW_SHOWNORMAL)
		}
	}))
	must(a.w.Bind("wmxOpenDataFolder", func() {
		_ = windows.ShellExecute(0, ptr("open"), ptr(a.dirs.Roaming), nil, nil, windows.SW_SHOWNORMAL)
	}))
	must(a.w.Bind("wmxDismissNotice", func() {
		a.mu.Lock()
		a.st.Notice = nil
		a.mu.Unlock()
	}))
}

// onReady runs when the page has rendered for the first time.
func (a *app) onReady() {
	a.mu.Lock()
	first := !a.ready
	a.ready = true
	a.mu.Unlock()
	a.push()
	if !first {
		return
	}
	visible := a.windowVisible()
	log.Printf("window ready (visible=%v)", visible)
	if a.opts.SelfTest != "" {
		_ = update.WriteJSON(a.opts.SelfTest, update.Health{Version: version.Version, PID: os.Getpid(), Visible: visible})
		go func() { time.Sleep(time.Second); a.w.Destroy() }()
		return
	}
	if p := a.opts.PostUpdate; p != "" {
		var plan update.Plan
		if update.ReadJSON(p, &plan) == nil && !update.TestFailHealth() {
			_ = update.WriteJSON(plan.Health, update.Health{Version: version.Version, PID: os.Getpid(), Visible: visible})
		}
		go a.cleanupAfterHelper(p)
	}
	if a.opts.UpdateFailed != "" {
		go a.cleanupAfterHelper(filepath.Join(a.dirs.Updates(), "pending.json"))
	}
	if update.TestAutoUpdate() && a.opts.PostUpdate == "" && a.opts.UpdateFailed == "" {
		go a.testAutoUpdate()
	}
}

// cleanupAfterHelper waits for the installer to exit, then removes downloads
// and staging files. The backup of the previous version stays.
func (a *app) cleanupAfterHelper(pendingPath string) {
	var plan update.Plan
	_ = update.ReadJSON(pendingPath, &plan)
	if plan.HelperPID != 0 && !update.WaitForExit(plan.HelperPID, 3*time.Minute) {
		log.Print("installer still running; leaving update files for the next start")
		return
	}
	if err := update.Cleanup(a.dirs.Updates(), exePath()); err != nil {
		log.Printf("cleanup: %v", err)
	} else {
		log.Print("update files cleaned up")
	}
}

func (a *app) loadNotices() {
	if a.opts.PostUpdate != "" {
		a.st.Notice = &notice{OK: true, Title: "Weightmaxer was updated to " + version.Version + "."}
	}
	if a.opts.UpdateFailed != "" {
		var out update.Outcome
		if update.ReadJSON(a.opts.UpdateFailed, &out) == nil {
			n := &notice{Title: "The update to " + out.To + " didn't finish.", Detail: out.Error + "."}
			if out.RolledBack {
				n.Detail += " Weightmaxer went back to version " + out.From + ", and your data is unchanged."
			}
			a.st.Notice = n
		}
	}
}

func (a *app) onCheckStart() {
	a.mu.Lock()
	a.st.Check.Checking = true
	a.mu.Unlock()
	a.push()
}

func (a *app) onCheckResult(r update.Result, manual bool) {
	a.mu.Lock()
	at := r.At
	a.st.Check.Checking = false
	a.st.Check.Kind, a.st.Check.Message, a.st.Check.At = r.Kind, r.Message, &at
	if !r.Kind.Failed() {
		a.st.Check.LastSuccess = &at
	}
	busy := a.st.Phase == "downloading" || a.st.Phase == "ready" || a.st.Phase == "installing"
	if r.Kind == update.KindAvailable && r.Release != nil && !busy {
		// A failed download stays visible until a newer version appears.
		if a.st.Phase != "error" || a.st.Latest != r.Release.Version {
			a.release = r.Release
			a.st.Latest = r.Release.Version
			a.st.Notes, a.st.More = update.WhatsNew(r.Release.Body)
			a.st.Phase, a.st.Error = "available", ""
		}
	} else if r.Kind == update.KindUpToDate && a.st.Phase == "available" {
		a.st.Phase = "idle"
	}
	a.mu.Unlock()
	log.Printf("update check: %s %s", r.Kind, r.Message)
	a.push()
}

// download runs only after the user presses Download.
func (a *app) download() {
	a.mu.Lock()
	rel := a.release
	if rel == nil || a.st.Phase == "downloading" || a.st.Phase == "installing" {
		a.mu.Unlock()
		return
	}
	a.st.Phase, a.st.Error, a.st.Progress = "downloading", "", progress{}
	a.mu.Unlock()
	a.push()
	fail := func(err error) {
		log.Printf("download failed: %v", err)
		a.mu.Lock()
		a.st.Phase, a.st.Error = "error", capitalize(err.Error())
		a.mu.Unlock()
		a.push()
	}
	if err := update.CheckWritable(filepath.Dir(exePath())); err != nil {
		fail(err)
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Minute)
	defer cancel()
	var lastPush time.Time
	zip, err := a.client.Download(ctx, rel, a.dirs.Updates(), func(done, total int64) {
		a.mu.Lock()
		a.st.Progress = progress{done, total}
		a.mu.Unlock()
		if time.Since(lastPush) > 250*time.Millisecond || done == total {
			lastPush = time.Now()
			a.push()
		}
	})
	if err != nil {
		fail(err)
		return
	}
	staging := filepath.Join(a.dirs.Updates(), "staging-"+rel.Version)
	_ = os.RemoveAll(staging)
	exe, err := update.ExtractExe(zip, rel.Version, staging)
	if err != nil {
		fail(err)
		return
	}
	log.Printf("downloaded and verified %s", rel.Version)
	a.mu.Lock()
	a.staged, a.st.Phase = exe, "ready"
	a.mu.Unlock()
	a.push()
}

// install runs only after the user presses "Install and restart".
func (a *app) install() error {
	a.mu.Lock()
	if a.st.Phase != "ready" || a.staged == "" || a.release == nil {
		a.mu.Unlock()
		return errors.New("Download the update first.")
	}
	staged, to := a.staged, a.release.Version
	a.mu.Unlock()
	for i := 0; a.store.Busy() && i < 40; i++ {
		time.Sleep(50 * time.Millisecond) // let a save in progress finish
	}
	if a.store.Busy() {
		return errors.New("Weightmaxer is still saving. Try again in a moment.")
	}
	if err := update.CheckExecutable(staged, to); err != nil {
		return err
	}
	target := exePath()
	plan := update.Plan{
		From: version.Version, To: to, Target: target, Staged: staged,
		BackupDir: a.dirs.Backup(), Health: filepath.Join(a.dirs.Updates(), "health.json"),
		Result: a.dirs.UpdateResult(), ParentPID: os.Getpid(), Created: time.Now(),
	}
	_ = os.Remove(plan.Result)
	if err := update.Start(plan, filepath.Join(a.dirs.Updates(), "pending.json")); err != nil {
		log.Printf("install failed to start: %v", err)
		return err
	}
	log.Printf("installing %s; closing", to)
	a.mu.Lock()
	a.st.Phase = "installing"
	a.mu.Unlock()
	a.push()
	go func() { time.Sleep(400 * time.Millisecond); a.w.Destroy() }()
	return nil
}

// testAutoUpdate drives check, download and install without clicks. It exists
// only in update test builds (see update.TestAutoUpdate).
func (a *app) testAutoUpdate() {
	r := a.client.Check(a.ctx)
	a.onCheckResult(r, false)
	if r.Kind != update.KindAvailable {
		return
	}
	a.download()
	a.w.Dispatch(func() {
		if err := a.install(); err != nil {
			log.Printf("test auto-update: %v", err)
		}
	})
}

// push sends the current state to the page (on the UI thread).
func (a *app) push() {
	a.mu.Lock()
	if !a.ready {
		a.mu.Unlock()
		return
	}
	b, _ := json.Marshal(a.st)
	a.mu.Unlock()
	a.w.Dispatch(func() { a.w.Eval("window.wmxUpdate && window.wmxUpdate(" + string(b) + ")") })
}

// watchFocus notices when the window comes back to the front.
func (a *app) watchFocus(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	was := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fg, _, _ := procGetForegroundWindow.Call()
			now := fg == a.hwnd
			if now && !was {
				a.sched.Focus()
			}
			was = now
		}
	}
}

func (a *app) windowActive() bool {
	iconic, _, _ := procIsIconic.Call(a.hwnd)
	return iconic == 0 && a.windowVisible()
}

func (a *app) windowVisible() bool {
	v, _, _ := procIsWindowVisible.Call(a.hwnd)
	return v != 0
}

func dpi() uint {
	if procGetDpiForSystem.Find() != nil {
		return 96
	}
	d, _, _ := procGetDpiForSystem.Call()
	if d < 96 {
		return 96
	}
	return uint(d)
}

// singleInstance returns false (after focusing the other window) if Weightmaxer is already open.
func singleInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Local\Weightmaxer-` + version.AppID)
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			windows.CloseHandle(h)
		}
		title, _ := windows.UTF16PtrFromString(windowTitle)
		if w, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title))); w != 0 {
			procShowWindow.Call(w, 9) // SW_RESTORE
			procSetForegroundWindow.Call(w)
		}
		return false
	}
	return true // the handle stays open for the life of the process
}

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func messageBox(text string) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(windowTitle)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10)
}

func openLog(path string) func() {
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags)
	return func() { f.Close() }
}

func ptr(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }

func capitalize(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
