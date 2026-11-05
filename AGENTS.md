# Weightmaxer: guide for contributors and AI tools

Read this file first, then `DEVELOPMENT_HANDOFF.md` and the shared standard in
[`docs/REPOSITORY_STANDARDS.md`](docs/REPOSITORY_STANDARDS.md). `CLAUDE.md` imports
this file; keep project rules here, not in per-tool files.

## What this is

A portable Windows desktop app (Go + WebView2) for planning weight loss or gain,
calorie and macro targets, and a weigh-in log. One public repository holds the
source and the releases; the app's update checker reads this repository's
GitHub releases without signing in.

## Folder map

| Path | Holds |
| --- | --- |
| `cmd/weightmaxer/` | Entry point; `--apply-update` runs the install helper |
| `cmd/wmxtool/` | `verify` (check a release zip) and `feedcheck` (live release check) |
| `internal/host/` | Windows window, WebView2 bindings, update UI state (Windows only) |
| `internal/update/` | Feed check, scheduler, download, verification, install helper, rollback |
| `internal/store/`, `internal/paths/`, `internal/version/` | Data file, per-user folders, version and identity marker |
| `web/` | The interface (HTML, CSS, JS) embedded into the executable |
| `assets/` | Icon source and PNG, Windows resource config, files shipped in the zip |
| `scripts/` | Build/package script, repository checks (`test_*.py`), icon renderer |
| `tests/browser/` | Playwright tests and the README screenshot capture script |
| `tests/e2e/` | Windows end-to-end update test |
| `docs/` | Standards, architecture, `releases/<version>.md`, `images/` |

The root is limited to the allowlist in `scripts/test_repository_layout.py`. A
genuinely required new root file is added to that list in the same pull request,
with the reason recorded here. Never relax it to a pattern.

## Commands

```bash
python3 scripts/build.py                                  # dist/ exe, zip, SHA256SUMS.txt
go test ./...                                             # unit tests (Linux or Windows)
python3 -m unittest discover -s scripts -p "test_*.py" -v # repository checks
python3 scripts/check_public_source.py                    # no secrets, binaries or private data
npm ci --prefix tests && npm test --prefix tests          # browser tests
go test ./tests/e2e -v -timeout 25m                       # Windows only: real update path
node tests/browser/screenshots.cjs                        # recapture docs/images/*.png
```

## Releases

- `VERSION` holds `MAJOR.MINOR.PATCH`. A release needs `docs/releases/<VERSION>.md`
  with `- ` bullets (the app shows the first four as "What's new").
- Merging to `main` runs CI; if all jobs pass and no release exists for `VERSION`,
  the `publish` job creates `v<VERSION>` with `Weightmaxer-win-x64.zip` and
  `SHA256SUMS.txt`, then downloads and verifies them. Existing releases are never
  replaced; bump `VERSION` for any new build.
- Documentation-only or organization-only changes don't bump `VERSION`.
- Release asset names, `Weightmaxer.exe` inside the zip, the identity marker
  format (`internal/version`) and the API path are an update contract with every
  installed copy. Changing them breaks updates for existing users.
- Approvals to push, merge or release are per task. Record them in the handoff
  as history; they don't carry over to later tasks.

## Gotchas

- The identity marker (`version.Identity`) is set with `-ldflags -X` and must stay
  referenced at runtime (it is logged at startup) or the linker drops it and the
  build fails its marker check.
- WebView2 must get `DataPath` under `%LOCALAPPDATA%`; without it the runtime
  creates a folder next to the executable.
- `go-webview2` runs bound functions on the UI thread. Long work (checks,
  downloads) goes to a goroutine; UI updates go back through `Dispatch`.
- Set a range input's `min`/`max`/`step` before its value, or the browser snaps
  the value to the old steps (this once changed the pace on unit switch).
- Test hooks (local feed, auto-update, failing health) exist only with the
  `updatetest` build tag. Release builds must not be built with it.
- Never auto-download or auto-restart. The banner buttons are the only triggers.
- The scheduler uses wall-clock time without the monotonic reading so time spent
  asleep counts toward the five-minute interval.
- Keep update checks within GitHub's 60 unauthenticated requests per hour per IP:
  ETag caching, pause on low remaining limit, honour `Retry-After`.
- The page scrolls as a whole. Don't add nested vertical scroll areas; the
  browser test fails on them.
- Screenshots use fictional data only and must be recaptured in the same pull
  request as a visible UI change.

## Repository standards (all contributors and AI tools)

This repository follows the shared TechSavage1989 standard in [`docs/REPOSITORY_STANDARDS.md`](docs/REPOSITORY_STANDARDS.md), the same file in every app repository. Read it before changing anything. In short:

- **README:** centered header (real icon, one `<h1>`, tagline, quick links Download · Get started · Features · Screenshots · Updates · Development, hero image), then the standard section order. Update it in the same pull request as any user-visible change.
- **Screenshots:** real captures with fictional data in `docs/images/` (`hero.png` + kebab-case names). Recapture any screenshot a UI change makes stale, in the same pull request.
- **Layout:** compact root; respect the root allowlist and checks above. Never commit build output, binaries, archives, logs, caches or secrets.
- **Branches:** short-lived `feat/` `fix/` `docs/` `chore/` `release/` branches, one focused change per pull request, merge only on green checks, delete the branch after merging.
- **Instructions:** this file is the single source of project rules for every tool; `CLAUDE.md` imports it. Add new rules and gotchas here.

Where this file's project-specific rules differ from the shared standard, this file wins; an enforced check script wins over both.
