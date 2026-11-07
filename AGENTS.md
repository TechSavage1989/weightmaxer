# Weightmaxer: guide for contributors and AI tools

Read this file first, then `DEVELOPMENT_HANDOFF.md` and the shared standard in
[`docs/REPOSITORY_STANDARDS.md`](docs/REPOSITORY_STANDARDS.md). `CLAUDE.md` imports
this file; keep project rules here, not in per-tool files.

## What this is

A portable Windows desktop app (Go + WebView2) for planning weight loss or gain,
calorie and macro targets, and a weigh-in log, plus the same interface as a
website on GitHub Pages at https://weightmaxer.techsavage3d.com/. One
public repository holds the source, the releases and the website; the app's
update checker reads this repository's GitHub releases without signing in.

Maintain both: every interface change in `web/` ships to the website on the next
merge to `main` and to the Windows app in its next release. Test both modes.

## Folder map

| Path | Holds |
| --- | --- |
| `cmd/weightmaxer/` | Entry point; `--apply-update` runs the install helper |
| `cmd/wmxtool/` | `verify` (check a release zip) and `feedcheck` (live release check) |
| `internal/host/` | Windows window, WebView2 bindings, update UI state (Windows only) |
| `internal/update/` | Feed check, scheduler, download, verification, install helper, rollback |
| `internal/store/`, `internal/paths/`, `internal/version/` | Data file, per-user folders, version and identity marker |
| `web/` | The interface (HTML, CSS, JS) embedded into the executable and published as the website |
| `assets/` | Icon source and PNG, Windows resource config, files shipped in the zip |
| `scripts/` | Build/package script, website build (`build_site.py`), repository checks (`test_*.py`), icon renderer |
| `tests/browser/` | Playwright tests and the README screenshot capture script |
| `tests/e2e/` | Windows end-to-end update test |
| `docs/` | Standards, architecture, `releases/<version>.md`, `images/` |

The root is limited to the allowlist in `scripts/test_repository_layout.py`. A
genuinely required new root file is added to that list in the same pull request,
with the reason recorded here. Never relax it to a pattern.

## Commands

```bash
python3 scripts/build.py                                  # dist/ exe, zip, SHA256SUMS.txt
python3 scripts/build_site.py                             # site/ for GitHub Pages
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

## Website

- `scripts/build_site.py` inlines `web/` into `site/index.html` and adds the icon and
  meta tags. CI tests the built site served over HTTP, then the `pages` job deploys
  it after all checks pass on a push to `main`. Pages uses the "GitHub Actions" source.
- The page is in desktop mode when the host's `wmxLoad` function exists, and in web
  mode otherwise. Mark elements `.desktop-only` (updates, folders) or `.web-only`
  (export/import, Windows download) and keep both modes covered by browser tests.
- Web mode stores the whole document in `localStorage["weightmaxer-data"]`. Keep the
  saved document format compatible with the desktop `data.json`, so an export from one
  imports into the other. Never add analytics, third-party scripts or remote fonts.
- The custom domain `weightmaxer.techsavage3d.com` is a CNAME to `techsavage1989.github.io`
  (DNS managed by the owner) and is set in Settings › Pages › Custom domain. Deployments
  from Actions ignore a `CNAME` file, so don't add one. `SITE_URL` in `scripts/build_site.py`
  must match it. Changing the domain again strands every visitor's browser-stored data at
  the old address (storage is per origin); warn users to export first.
- Website-only changes don't need a `VERSION` bump. A change that alters the Windows
  app's behaviour does.

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
- The website is used on phones. Keep it working at 390 px wide: the browser test
  "phone width" fails if the page or a table needs side-scrolling, a tab wraps, a control
  sticks out of its card, BMI or chart labels overlap (checked over many start/goal pairs), or a text field is under 16 px (Safari
  zooms in on focus). iPhone Safari date fields have a built-in minimum width; keep the
  `@supports (-webkit-touch-callout: none)` rules in `web/app.css`. Headless Chromium can't
  reproduce that, so check real-phone reports against screenshots.
- Theme: `data.theme` is `system`, `light` or `dark`; `light`/`dark` set `data-theme` on
  `<html>`, and every colour is a token that both `@media (prefers-color-scheme: dark)` and
  `[data-theme="dark"]` redefine. A small script in `index.html` applies the saved web
  choice before first paint. Don't add colours outside the tokens.
- Chart time labels go through `xTicks` and the collision filter in `drawChart`; the first
  and last labels always show. The overlap test runs in both Weeks and Dates modes.
- Chart readout: `drawChart` stores its scales on `svg._chart` and draws a hidden `.hover`
  group; `showHover` positions it and the `.chart-tip` box (inside `.chart-box`) for a day
  number. Pointer, touch and keyboard events go through `attachHover`. Keep the readout
  inside the chart box and the plan value equal to the schedule (both tested).
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
