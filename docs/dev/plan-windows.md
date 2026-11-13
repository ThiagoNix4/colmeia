# Plan — Windows beta (private)

goal: Colmeia runs on Windows 10/11 x64; dist = portable `.exe` (≈AppImage) + NSIS installer (≈deb). Private prerelease on GitHub, user tests on own Windows PC. Linux must keep working (no regressions).

## Contract (both sides MUST match)
- transport win32: named pipe `\\.\pipe\colmeia-<USERNAME>` (override `COLMEIA_SOCK_PATH`). Linux unchanged (unix sock).
- pipe ACL: current user only (SDDL `D:P(A;;GA;;;<user SID>)` or owner-only). Token auth stays (`COLMEIA_TOKEN`); SO_PEERCRED check = unix only.
- daemon bin: `out/colmeiad.exe` on win, same flags (`--docs --cli --sock --version`).
- config dir: same rule both sides (`XDG_CONFIG_HOME` || `~/.config/colmeia`) — keep, no APPDATA move in beta.
- CLI in cards: daemon prepends cliDir to PATH → `cli/colmeia.cmd` must exist (`@node "%~dp0colmeia.mjs" %*`).
- card shell win32: `pwsh.exe` if in PATH else `powershell.exe` (`COLMEIA_SHELL` override). Preset cmd: `-NoLogo -Command <script>`; KeepShell → `-NoLogo -NoExit -Command <script>`. Plain shell card: `-NoLogo`.
- ManagerCommand win32: read ORCHESTRATOR.md w/o bash (`$(Get-Content -Raw '{docs}\ORCHESTRATOR.md')`); beware PS 5.1 native-arg quoting of embedded `"` → verify/escape or use file-based flag if tool supports.

## T1 — daemon Windows port · worker GoTerm (Sonnet) · files: `daemon/**` only
- build tags `_unix.go` / `_windows.go`; no `golang.org/x/sys/unix` in shared files
- PTY: ConPTY on win (prefer `github.com/charmbracelet/x/xpty` or `conpty`); creack/pty stays on unix. Resize, write, read, exit code.
- listener: go-winio named pipe on win; dial-check of existing daemon same
- main.go: flock/pid lock → win equivalent (pipe already in use = running); prctl subreaper, SIGHUP group kill → unix only; win: kill tree via job object (preferred) or `taskkill /T /F`
- reaper.go unix only; auth.Peer unix only (win: skip, rely on pipe ACL + token)
- spawn.go/store.go: shell + command per contract above
- done: `go vet ./... && go test ./...` (linux) ✓, `GOOS=windows GOARCH=amd64 go build ./... && go vet` ✓

## T2 — Electron/CLI/packaging · worker AuditFront (Sonnet) · files: `electron/**`, `cli/**`, `package.json`, `scripts/**`, `src/**` (only if needed)
- sock path win32 per contract in `electron/main.ts`, `electron/daemonClient.ts`, `cli/colmeia.mjs`, `cli/colmeia-statusline.mjs`; no `fs.existsSync(pipe)` waits on win
- daemon bin name `.exe`; detached spawn ok on win (windowsHide)
- `cli/colmeia.cmd` wrapper
- package.json: `build:daemon:win` (`GOOS=windows GOARCH=amd64 go build -o ../out/colmeiad.exe`), `dist:win` → electron-builder `--win portable nsis --x64`; `win.icon` (assets/icon.png ok, ≥256px), asarUnpack `out/colmeiad.exe`; files must not ship linux colmeiad in win build and vice versa if easy
- if electron-builder needs wine → stop, report (user installs)
- done: `npx tsc --noEmit` ✓, `npm run build` ✓ (linux), `npm run dist:win` produces both in dist/

## T3 — review · ReviewF1 (Opus) after T1+T2 → `docs/dev/review-windows.md` (H/M/L), focus: contract mismatch, quoting, process cleanup, linux regressions
## T4 — release · manager: prerelease `v0.2.0-beta.1` (private repo), assets win only, notes = how to test + known limits
