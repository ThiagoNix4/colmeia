# Review — Windows port (T3)

scope: uncommitted diff + untracked (T1 daemon, T2 electron/cli/pkg). checks: `go vet` linux ✓, `GOOS=windows go vet` ✓, `go test ./...` ✓, `tsc --noEmit` ✓, `node --check cli/colmeia.mjs` ✓.
verdict: Linux path ≈ identical (refactor 1:1). Windows: 3 blockers (H) before beta.

## H — blockers

H1 · daemon default sock ≠ contract → daemon never starts on win
- `daemon/cmd/colmeiad/main.go:81-89`: fallback = `%TEMP%\colmeia.sock`; contract = `\\.\pipe\colmeia-<USERNAME>`.
- Electron spawns w/o `--sock` and w/o `COLMEIA_SOCK_PATH` (`electron/main.ts:113-116`, `electron/daemonClient.ts:154`) → `winio.ListenPipe` gets a file path → invalid pipe name → exit 1 → "colmeiad não respondeu".
- fix: win default in main.go = `\\.\pipe\colmeia-`+username (same as `os.userInfo().username`: user part of `user.Current().Username` after `\`, or `%USERNAME%`), and/or Electron passes `--sock socketPath` (both sides; safest).

H2 · ConPTY read never EOFs → dead cards stay "alive"
- `daemon/internal/term/manager.go:217-239`: readLoop blocks on `ptmx.Read` until error, only then `proc.Wait()` + `ptmx.Close()`.
- conpty v0.2.0 keeps `outPipeWrite` + hPC open in daemon until `Close()` (`conpty_windows.go:125-137`) → Read never returns after shell exits → `alive` never false, no exit event, `done` never closed; `terminate()` waits 2.5s for nothing; preset w/o KeepShell never ends.
- fix (pty_windows.go only): in startProc spawn goroutine `WaitForSingleObject(h)` → `cp.Close()` (ClosePseudoConsole flushes then breaks pipe) so Read gets EOF; Wait then returns cached code. Or close `inPipeRead/outPipeWrite` right after Spawn + ClosePseudoConsole on exit.

H3 · PATH dup on win → cliDir dropped → `colmeia` not found in cards
- `daemon/internal/api/spawn.go:114-123`: env copies `Path=…` (Windows spelling) then sets `PATH=cliDir;…`.
- `manager.go:166-173` sorts keys → `PATH` < `Path`; conpty `dedupEnvCase(true)` keeps LAST → original `Path` wins, cliDir lost. CLI + `colmeia.cmd` unreachable in every card.
- fix: on win look up existing key case-insensitively (`strings.EqualFold(k,"PATH")`) and overwrite that key; same care for any env override.

## M

M1 · no graceful shutdown on win (history/FlushAll lost, stale pidfile)
- `main.go:118` waits SIGTERM/SIGINT/SIGHUP; detached no-console proc never receives them on win. `stopPid` = `taskkill /T /F` (`main_windows.go:40-42`); Electron restart path uses `--stop` (`daemonClient.ts:193`).
- → `srv.Close/FlushAll/clearPid` never run; last scrollback unflushed; `colmeiad.pid` stays.
- stale pid + `--stop` later → `taskkill /T /F` on reused PID = kills unrelated process tree.
- fix: graceful stop via pipe RPC (e.g. `daemon.stop` method or named event) before taskkill fallback; on `--stop` verify pid image name == colmeiad.exe (or lock file held) before taskkill.

M2 · Manager cmd encoding: `Get-Content -Raw` w/o `-Encoding UTF8`
- `daemon/internal/store/store.go:159`: PS 5.1 reads ANSI (cp1252) → PT accents in ORCHESTRATOR.md mojibake in system prompt. fix: `-Encoding UTF8` (valid 5.1 + 7).

M3 · Manager cmd via npm `claude.cmd` shim breaks
- same line: if `claude` resolves to `claude.cmd` (npm global install), arg goes through cmd.exe: 8.9 KB prompt with newlines/`&|<>%` → truncated at 1st newline / cmd 8191-char limit; PS 7.3+ `PSNativeCommandArgumentPassing=Windows` also uses legacy quoting for .cmd → version check `-lt 7.3` wrong for that case.
- fix: prefer file-based flag if available (`--append-system-prompt-file`?) else document "native claude.exe required" in beta notes; at least escape when target is .cmd.

M4 · NSIS install: daemon outlives app and locks its exe
- `electron/main.ts:90` stages only for AppImage/portable; NSIS runs `colmeiad.exe` from `Program Files\…\app.asar.unpacked`. Daemon survives close → NSIS upgrade/uninstall fails "file in use".
- fix: also stage on win32 always (`process.platform==='win32'`), or NSIS `customInit` runs `colmeiad.exe --stop`.

M5 · win kill is always hard; `Terminate(false)` == TerminateJobObject
- `daemon/internal/term/pty_windows.go:62-74`: no soft phase (≈SIGHUP) → claude/agents killed w/o cleanup on card close. Acceptable beta, but try `GenerateConsoleCtrlEvent`/write `\x03` to ConPTY first, then job kill after 1.5s (manager already has the delay). → list as known limit if not fixed.

## L

L1 · `pty_windows.go:54`: Wait kills job after shell exit → background procs started in card die (Linux: survive via subreaper). Behaviour diff; document.
L2 · `pty_windows.go:103`: process handle `h` never closed (leak/card); job assigned after spawn (child can escape in tiny window; use CREATE_SUSPENDED if conpty allows).
L3 · `daemon/internal/api/listen_windows.go:21-23`: if `user.Current()` fails → no SDDL → default pipe DACL (Everyone read). Fail closed instead.
L4 · `daemon/internal/auth/peer_windows.go:9`: pid 0 → always External; Orphan rejection (unix M2) absent on win; token-only auth. OK per contract, document.
L5 · `store.go:159`: manual escape `"`→`\"` ignores backslashes before quotes / trailing `\` (CommandLineToArgvW rules). Low risk with current ORCHESTRATOR.md.
L6 · `{docs}` substituted raw inside `'…'` (`spawn.go:98`, store.go:159): path with `'` breaks (pre-existing on unix too).
L7 · `electron/daemonClient.ts:154`: daemon spawned w/o `cwd` → inherits Electron cwd (portable temp extract dir) → dir locked while daemon lives; also feeds `firstExisting(wd/docs)`. Set `cwd: configDir()`.
L8 · `package.json` `build:daemon:win` uses POSIX `GOOS=… go build` → cross-build from Linux only (fine per plan; won't run on Windows cmd).
L9 · `main_unix.go:16-20`: OpenFile error now retried 5s then "já rodando" exit 0 (before: "pidfile:" + exit 1). Minor Linux behaviour change in error path only.
L10 · daemon survival on app close: `detached:true` → libuv skips its kill-on-close job ✓; only risk = Electron itself launched inside a job w/o breakaway. Verify on real PC.

## Linux regressions
none found in normal path: shQuote/args/`-l -i -c`/KeepShell `exec $SHELL -l` identical; flock+inode check, subreaper, SIGHUP→SIGKILL group kill, peer creds, reaper (`//go:build !windows`) preserved; electron sock/bin path unchanged on linux; `linux.files` excludes `.exe` ✓.

## Round 2

checks: `go vet` linux ✓, `GOOS=windows go vet` ✓, `go test ./...` ✓, `tsc --noEmit` ✓.

### fixed items
- H1 ✓ `main_windows.go:52-61` defaultSock = `\\.\pipe\colmeia-<user>` (domain stripped ≈ `os.userInfo().username`); Electron also passes `--sock` on win (`electron/main.ts:116`). belt+braces.
- H2 ✓ `pty_windows.go:122-126` goroutine waits process → 300ms → `cp.Close()` → Read EOF. order safe: Wait (closes `h`) only runs after EOF, i.e. after goroutine is done.
- H3 ✓ `spawn.go:123-130` overwrites existing `Path` key case-insensitively; linux = same `PATH` key, identical.
- M1 ✓ named event `Local\colmeiad-stop` → SIGTERM on ch → normal flush/clearPid (`main_windows.go:64-75`); `--stop` checks lock, waits 5s, taskkill only if image == colmeiad.exe (`:79-115`). stale-pid kill fixed.
- M2 ✓ `-Encoding UTF8` (`store.go` winPromptArg).
- M3 ✓ .cmd/.bat shim → short pointer to file; PS<7.3 escaping now CommandLineToArgvW-correct (backslash runs + trailing `\`). L5 closed too.
- M4 ✓ win32 always stages to `~/.local/share/colmeia/<ver>` (`electron/main.ts:90`) → installer exe no longer locked.
- M5 ✓ soft = `0x03` to ConPTY, hard after 1.5s by manager (`pty_windows.go:68-75`).
- L2 ✓ process handle closed in Wait. L3 ✓ fail closed w/o SID. L7 ✓ `cwd: configDir()`. L9 ✓ real OpenFile error → exit 1 again (unix flock errors → errBusy, = old behaviour).

### new findings (no H, no M)
- L11 · stop event name global per session (`main_windows.go:49`), not per config dir/pipe: 2 daemons (dev + prod via `XDG_CONFIG_HOME`/`COLMEIA_SOCK_PATH`) share one auto-reset event → `--stop` may wake the wrong one. fix: suffix with hash of pidf or sock.
- L12 · `main.go:122` notifyStop runs after `srv.Start`; `--stop` during startup window → OpenEvent fails → falls to taskkill (still guarded by isColmeiad). benign.
- L13 · Electron `execFileSync(--stop, timeout 5000)` (`daemonClient.ts:193`) vs stopPid up to 5s wait + taskkill → may time out; caught, next loop waits on oldPid → ok, just slower.
- L14 · win32 staging keyed only by `app.getVersion()` (`electron/main.ts:69-70`): same-version rebuild (re-uploaded beta asset, or `npm run dev` on win) reuses stale staged daemon/cli/docs. pre-existing for AppImage; on win now affects every build. fix: key by binVersion hash or bump version per asset.
- L15 · M3 detection uses `Get-Command claude`: npm also ships `claude.ps1`, which PS picks first → full prompt path (fine), but under default ExecutionPolicy Restricted it errors "scripts disabled". user env; note in release "known limits" (prefer native claude.exe).
- L16 · `pty_windows.go:119` soft Ctrl-C in pwsh only cancels line; card with idle pwsh always dies by hard kill after 1.5s → close latency 1.5s on win. cosmetic.

### Linux regressions
none: unix branches identical (sock default moved to `main_unix.go:46` unchanged; `--stop` = SIGTERM; notifyStop no-op; PATH key = `PATH`; staging condition unchanged off win32).
