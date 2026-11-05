//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Start begins installing: it copies the verified executable to a staging
// folder, writes the plan and starts that copy as the helper. The caller must
// exit right after Start returns nil so the helper can replace it.
func Start(p Plan, pendingPath string) error {
	if err := WriteJSON(pendingPath, p); err != nil {
		return err
	}
	cmd := exec.Command(p.Staged, "--apply-update="+pendingPath)
	cmd.Dir = filepath.Dir(p.Staged)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't start the installer: %w", err)
	}
	return cmd.Process.Release()
}

// RunHelper is the --apply-update entry point. It waits for the old app to
// exit, keeps a backup, swaps in the new executable, starts it and waits for a
// visible window. On any failure it restores the backup and starts the old
// version again.
func RunHelper(pendingPath string) int {
	var p Plan
	if err := ReadJSON(pendingPath, &p); err != nil {
		return 2
	}
	p.HelperPID = os.Getpid()
	_ = WriteJSON(pendingPath, p)
	out := Outcome{From: p.From, To: p.To}
	finish := func(err error, rolledBack bool) int {
		out.OK, out.RolledBack, out.Finished = err == nil, rolledBack, time.Now()
		if err != nil {
			out.Error = err.Error()
		}
		_ = WriteJSON(p.Result, out)
		if err != nil {
			return 1
		}
		return 0
	}
	if !waitPID(p.ParentPID, 60*time.Second) {
		return finish(errors.New("Weightmaxer didn't close, so the update wasn't installed"), false)
	}
	backup, err := keepOneBackup(p.Target, p.BackupDir, p.From)
	if err != nil {
		_ = launch(p.Target, "--update-failed="+p.Result)
		return finish(err, false)
	}
	if err := swapIn(p.Staged, p.Target); err != nil {
		_ = launch(p.Target, "--update-failed="+p.Result)
		return finish(fmt.Errorf("couldn't replace Weightmaxer.exe: %w", err), false)
	}
	os.Remove(p.Health)
	proc, err := launchProc(p.Target, "--post-update="+pendingPath)
	if err == nil {
		err = waitHealthy(p, proc)
	}
	if err == nil {
		return finish(nil, false)
	}
	// Roll back.
	if proc != nil {
		_ = proc.Kill()
		_, _ = proc.Wait()
	}
	if rerr := swapIn(backup, p.Target); rerr != nil {
		return finish(fmt.Errorf("%v; restoring the previous version also failed: %v (a copy is in %s)", err, rerr, backup), false)
	}
	out.Error = err.Error()
	out.RolledBack = true
	out.Finished = time.Now()
	_ = WriteJSON(p.Result, out)
	_ = launch(p.Target, "--update-failed="+p.Result)
	return 1
}

// swapIn copies src next to dst (same folder, so the final step is a rename
// on one volume even if src is elsewhere) and atomically replaces dst.
func swapIn(src, dst string) error {
	next := dst + ".new"
	os.Remove(next)
	if err := copyFile(src, next); err != nil {
		return err
	}
	from, _ := windows.UTF16PtrFromString(next)
	to, _ := windows.UTF16PtrFromString(dst)
	var err error
	// Antivirus scanners briefly hold new executables open; retry for a while.
	for i := 0; i < 40; i++ {
		err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	os.Remove(next)
	return err
}

func waitHealthy(p Plan, proc *os.Process) error {
	exited := make(chan struct{})
	go func() { _, _ = proc.Wait(); close(exited) }()
	deadline := time.Now().Add(HealthTimeout)
	for time.Now().Before(deadline) {
		var h Health
		if ReadJSON(p.Health, &h) == nil && h.PID == proc.Pid && h.Version == p.To && h.Visible {
			return nil
		}
		select {
		case <-exited:
			return fmt.Errorf("Weightmaxer %s closed right after starting", p.To)
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("Weightmaxer %s didn't open its window", p.To)
}

func launch(path string, args ...string) error {
	p, err := launchProc(path, args...)
	if err == nil {
		_ = p.Release()
	}
	return err
}

func launchProc(path string, args ...string) (*os.Process, error) {
	cmd := exec.Command(path, args...)
	cmd.Dir = filepath.Dir(path)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

// waitPID waits for a process to exit. A process that is already gone counts as exited.
func waitPID(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	return ev == windows.WAIT_OBJECT_0
}

// WaitForExit is used by the new app to wait for the helper before cleaning up.
func WaitForExit(pid int, timeout time.Duration) bool { return waitPID(pid, timeout) }
