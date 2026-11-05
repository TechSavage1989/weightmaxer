# Development handoff

Read [`AGENTS.md`](AGENTS.md) first. This file records the current state for the next session.

## Current state

- **Version:** 1.0.0 (first release), from `VERSION`.
- **Repository and update feed:** `TechSavage1989/weightmaxer` (public). Source and
  releases live here; installed copies read
  `https://api.github.com/repos/TechSavage1989/weightmaxer/releases/latest`.
- **Release assets:** `Weightmaxer-win-x64.zip` (contains `Weightmaxer.exe`,
  `README.txt`, `THIRD_PARTY_NOTICES.txt`) and `SHA256SUMS.txt`.
- **Platform:** Windows 10/11 x64 with the WebView2 Runtime. Built by cross-compiling
  with Go; no C toolchain.

## Build and test

| Step | Command | Where it runs |
| --- | --- | --- |
| Release build and package | `python3 scripts/build.py` | Linux CI, any OS locally |
| Reproducibility | build twice, `cmp` the zips | Linux CI |
| Unit tests | `go test ./...` | Linux CI and Windows CI |
| Repository checks | `python3 -m unittest discover -s scripts -p "test_*.py" -v`, `python3 scripts/check_public_source.py` | Linux CI |
| Browser tests | `npm ci --prefix tests && npm test --prefix tests` | Linux CI |
| Release exe opens its window | `Weightmaxer.exe --self-test=<file>` | Windows CI |
| End-to-end update | `go test ./tests/e2e -v -timeout 25m` | Windows CI |
| Live feed after publishing | `go run ./cmd/wmxtool feedcheck --expect <version>` | publish job |

## Settings and data paths

- `%APPDATA%\Weightmaxer\data.json` (+ `data.json.bak`): plan, weigh-ins, units, diet style.
- `%LOCALAPPDATA%\Weightmaxer\`: `WebView2\`, `logs\weightmaxer.log` (rotates at 1 MB),
  `update-cache.json`, `updates\` (downloads, staging, `pending.json`, `result.json`),
  `backup\` (exactly one previous version).
- Nothing persistent is written beside `Weightmaxer.exe`. During an install a
  short-lived `<exe>.new` exists next to it and is renamed over the target.

## Known limitations

- `Weightmaxer.exe` is not code-signed. SmartScreen may warn on first start, and a
  matching SHA-256 shows the file came from this repository's release, not who signed it.
- The app can't update itself from a read-only folder such as Program Files; it says so
  and doesn't ask for elevation.
- "Online" is detected from failed requests (backoff), not from Windows network events.
- Focus return is detected by polling the foreground window once a second.
- README screenshots are captured from the real interface (`web/index.html`) in Chromium,
  the engine WebView2 uses, not from a screenshot of the Windows window.
- No manual testing on a Windows 10 or 11 desktop has been done yet. Windows checks run
  on GitHub's hosted Windows Server runner.

## Documentation

- README follows the shared standard; `scripts/test_release_metadata.py` checks its
  heading anchors, quick links, image paths and that downloads link `releases/latest`.
- `docs/images/icon.png` must equal `assets/icon.png` (checked); regenerate both with
  `node scripts/render_icon.cjs`. Recapture screenshots with `node tests/browser/screenshots.cjs`.
- No documentation exceptions to the shared standard.

## Branches

- `main`: released code. Work happens on short-lived `feat/`, `fix/`, `docs/`, `chore/`
  or `release/` branches merged by pull request on green checks, then deleted.
- No other branches carry unmerged work.

## Task history

### 2026-11-05: initial app and 1.0.0 release

- Requested: a Windows desktop app built from the earlier weight-loss calculator, with
  reliable updates and a handoff. Added during the task: calorie minimums below 1,500 kcal
  (user-set, 800 kcal floor, with a warning), weight-gain planning, and macro recommendations.
- Approvals given for this task only: source upload and public publication to
  `TechSavage1989/weightmaxer`, pull request creation, merge into `main` after checks,
  and the `v1.0.0` tag and release with its assets. They don't authorize later work.
- Results are recorded in the pull request and the release.
