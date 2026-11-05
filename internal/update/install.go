package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Plan describes one installation, shared by the running app, the helper and
// the relaunched app through updates\pending.json.
type Plan struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Target    string    `json:"target"`     // the running executable, whatever the user named it
	Staged    string    `json:"staged"`     // verified new Weightmaxer.exe in %LOCALAPPDATA%
	BackupDir string    `json:"backup_dir"` // one previous version is kept here
	Health    string    `json:"health"`     // written by the new app once its window is visible
	Result    string    `json:"result"`     // written by the helper
	ParentPID int       `json:"parent_pid"`
	HelperPID int       `json:"helper_pid,omitempty"`
	Created   time.Time `json:"created"`
}

// Outcome is the helper's report.
type Outcome struct {
	OK         bool      `json:"ok"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Error      string    `json:"error,omitempty"`
	RolledBack bool      `json:"rolled_back,omitempty"`
	Finished   time.Time `json:"finished"`
}

// Health is written by a freshly started app once its window is showing.
type Health struct {
	Version string `json:"version"`
	PID     int    `json:"pid"`
	Visible bool   `json:"visible"`
}

// HealthTimeout is how long the helper waits for the new window.
var HealthTimeout = 90 * time.Second

func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// CheckWritable confirms new files can be created next to the executable.
// Weightmaxer never asks for administrator rights; a copy in a protected
// folder such as Program Files has to be updated by downloading it again.
func CheckWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".weightmaxer-write-test-*")
	if err != nil {
		return fmt.Errorf("Weightmaxer can't update itself in %s. Move Weightmaxer.exe to a folder you can change, such as Documents, and start it from there", dir)
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// copyFile copies src to dst through a temporary file and flushes it to disk.
func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// keepOneBackup copies target into dir as Weightmaxer-<version>.exe and
// removes any older backups.
func keepOneBackup(target, dir, ver string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "Weightmaxer-"+ver+".exe")
	if err := copyFile(target, path); err != nil {
		return "", fmt.Errorf("couldn't back up the current version: %w", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if p := filepath.Join(dir, e.Name()); p != path && strings.HasPrefix(e.Name(), "Weightmaxer-") {
			os.Remove(p)
		}
	}
	return path, nil
}

// Cleanup removes app-owned update leftovers: downloads, staging folders and
// pending files. It keeps result.json and the backup folder. Call it only once
// the new version is healthy and the helper has exited.
func Cleanup(updatesDir, target string) error {
	entries, err := os.ReadDir(updatesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var firstErr error
	for _, e := range entries {
		n := e.Name()
		if n == "result.json" {
			continue
		}
		if strings.HasPrefix(n, "staging-") || strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".part") ||
			strings.HasSuffix(n, ".tmp") || n == "pending.json" || n == "health.json" {
			if err := os.RemoveAll(filepath.Join(updatesDir, n)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if target != "" {
		for _, suffix := range []string{".new", ".new.tmp"} {
			os.Remove(target + suffix)
		}
	}
	return firstErr
}
