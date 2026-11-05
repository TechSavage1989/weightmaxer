// Package paths decides where Weightmaxer keeps per-user files. Nothing is
// written beside the executable except a short-lived replacement file while an
// update is being installed.
package paths

import (
	"errors"
	"os"
	"path/filepath"
)

// Dirs holds the per-user folders.
type Dirs struct {
	// Roaming holds settings and the user's plan and weigh-in log
	// (%APPDATA%\Weightmaxer on Windows).
	Roaming string
	// Local holds machine-specific files: WebView2 data, logs, the update cache,
	// downloads, staging and the rollback backup (%LOCALAPPDATA%\Weightmaxer).
	Local string
}

const appFolder = "Weightmaxer"

// Default returns the standard per-user folders for the current OS user.
func Default() (Dirs, error) {
	if home := testHome(); home != "" {
		return Under(home), nil
	}
	roaming, err := os.UserConfigDir() // %APPDATA% on Windows
	if err != nil {
		return Dirs{}, err
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		if local, err = os.UserCacheDir(); err != nil {
			return Dirs{}, err
		}
	}
	if roaming == "" || local == "" {
		return Dirs{}, errors.New("no per-user application folder is available")
	}
	return Dirs{Roaming: filepath.Join(roaming, appFolder), Local: filepath.Join(local, appFolder)}, nil
}

// Under places both folders inside root; used by tests.
func Under(root string) Dirs {
	return Dirs{Roaming: filepath.Join(root, "Roaming", appFolder), Local: filepath.Join(root, "Local", appFolder)}
}

func (d Dirs) DataFile() string     { return filepath.Join(d.Roaming, "data.json") }
func (d Dirs) WebViewData() string  { return filepath.Join(d.Local, "WebView2") }
func (d Dirs) LogFile() string      { return filepath.Join(d.Local, "logs", "weightmaxer.log") }
func (d Dirs) UpdateCache() string  { return filepath.Join(d.Local, "update-cache.json") }
func (d Dirs) Updates() string      { return filepath.Join(d.Local, "updates") }
func (d Dirs) Backup() string       { return filepath.Join(d.Local, "backup") }
func (d Dirs) UpdateResult() string { return filepath.Join(d.Local, "updates", "result.json") }

// Ensure creates both folders.
func (d Dirs) Ensure() error {
	for _, p := range []string{d.Roaming, d.Local, filepath.Dir(d.LogFile())} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return err
		}
	}
	return nil
}
