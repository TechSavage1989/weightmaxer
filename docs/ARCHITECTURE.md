# Architecture

## Window

`cmd/weightmaxer` calls `internal/host.Run`, which creates a WebView2 window
(go-webview2, no C compiler needed) and loads `web/index.html` with its CSS and
JS inlined from memory (`web.Page`). The page talks to Go through bound
functions:

| Function | Purpose |
| --- | --- |
| `wmxLoad`, `wmxSave` | Read and write `%APPDATA%\Weightmaxer\data.json` atomically (`internal/store`) |
| `wmxInfo`, `wmxReady` | Version and data folder; first-render signal used for health reports |
| `wmxCheckNow`, `wmxDownload`, `wmxInstall` | The update buttons |
| `wmxOpenNotes`, `wmxOpenDataFolder`, `wmxDismissNotice` | Small helpers |

Go pushes update state to `window.wmxUpdate(state)`.

A named mutex keeps one copy running; a second start brings the first window forward.

## Update flow

1. **Check** (`update.Client.Check`): `GET /repos/TechSavage1989/weightmaxer/releases/latest`
   with `If-None-Match` from `%LOCALAPPDATA%\Weightmaxer\update-cache.json`. 304 reuses
   the cached body. 404 means no release yet. 403/429 with rate-limit headers or
   `Retry-After` pause checks; fewer than five remaining requests pause automatic
   checks until the reset. Drafts, prereleases, bad tags and missing assets are
   errors, never "up to date".
2. **Schedule** (`update.Scheduler`): check at startup, every 5 minutes while the
   window is visible and not minimized, on focus return after 2 minutes, and on
   "Check now" (60-second cooldown). One check at a time. Failures back off
   10, 20, 40, then 60 minutes. Focus is detected by polling the foreground window
   once a second.
3. **Download** (user action): asset URLs must be exactly
   `https://github.com/TechSavage1989/weightmaxer/releases/download/v<version>/<name>`.
   Every request and redirect must be HTTPS to an allowlisted GitHub host. The
   zip's SHA-256 must match `SHA256SUMS.txt` and GitHub's asset digest when
   present; the size must match the release. Files go to `%LOCALAPPDATA%\Weightmaxer\updates\`.
4. **Verify** (`update.ExtractExe`): at most 8 entries, only `Weightmaxer.exe`,
   `README.txt`, `THIRD_PARTY_NOTICES.txt`; no paths, links, duplicates, oversized
   or over-compressed entries. The executable must be PE32+ AMD64, a GUI program,
   and contain `WMXID|com.techsavage1989.weightmaxer|<version>|END`.
5. **Install** (user action): the app writes `updates\pending.json`, starts the
   staged executable with `--apply-update`, and exits. The helper waits for the
   old process, copies the current executable to `backup\Weightmaxer-<old>.exe`
   (deleting older backups), copies the new one next to the target as `.new` (same
   volume) and replaces the target with `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)`,
   retrying while antivirus holds the file. It starts the new version with
   `--post-update` and waits up to 90 seconds for `updates\health.json` showing the
   new version, its PID and a visible window.
6. **Rollback**: if the swap fails, the new version exits, or no healthy report
   arrives, the helper ends the new process, restores the backup the same way and
   starts the old version with `--update-failed`, which shows why.
7. **Cleanup**: the new version waits for the helper to exit, then deletes
   downloads, staging folders and pending files. The backup and `result.json` stay.

## Test builds

The `updatetest` build tag adds environment hooks used only by `tests/e2e`:
`WEIGHTMAXER_TEST_HOME` (all folders), `WEIGHTMAXER_TEST_API` and
`WEIGHTMAXER_TEST_CA` (local HTTPS feed), `WEIGHTMAXER_TEST_AUTO_UPDATE`,
`WEIGHTMAXER_TEST_FAIL_HEALTH` and `WEIGHTMAXER_TEST_HEALTH_TIMEOUT`.
Release builds compile none of this.
