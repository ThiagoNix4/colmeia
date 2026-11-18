# Review — melhorias 4 (notes seed + --app-version)

Scope: `daemon/internal/api/notes_seed.go` (+ test, grid positions, mutex) and the `--app-version` flow
`electron/main.ts` → `colmeiad` ping `appVersion` → `colmeia --version`.

## OK

- `notes_seed.go`: `seedMu` covers the whole check (`notes.List` / cards / edges) plus all 4 `notes.Create` + `notes.Update`,
  so concurrent `ws.load` cannot seed twice. `notes.Create`/`Update` have their own `notes.mu`; no deadlock (seedMu is outer, never taken inside notes).
- Caller `ui.go:219` only seeds when `isNew` (computed before `LoadWorkspace`), outside `s.mu` — no lock-order issue.
- Grid: x = (i%2)*460, y = (i/2)*360 with notes 420x320 → 4 distinct positions, 40px gap, no overlap.
- Test `TestSeedNotesDistinctAndIdempotent` covers idempotency, distinct id/file/position, and body starting with `# <title>`.
- `cli/colmeia.mjs:345-357`: reads package.json first; ping fallback handles a daemon without `appVersion` (prints the hash only).

## Bugs

### 1. `electron/main.ts:154` — an old daemon fails to start because of the `--app-version` flag

- **Bug:** `--app-version` goes on argv. A `colmeiad` built before this change exits with code 2
  (`flag provided but not defined: -app-version`). This happens when:
  - Linux/AppImage: the staged folder is named after `app.getVersion()` only (`main.ts:84`), so after a rebuild
    with the same app version, `stageAppImage` reuses the existing `~/.local/share/colmeia/<version>/colmeiad`, which is the old binary.
  - Dev: `COLMEIA_DAEMON_BIN` points to an old build, or `out/colmeiad` is stale.
  
  Before this change these cases only meant a stale daemon. Now `spawnDaemon` fails silently (the error goes to `colmeiad.log`), `connect()` times out, and the app hits `fatal` and exits.
- **Fix:** pass the version through the environment, not argv. The daemon already defaults the flag to `os.Getenv("COLMEIA_APP_VERSION")`.
  Remove `'--app-version', app.getVersion()` from `spawnArgs`, and in `daemonClient.ts:151` spawn with
  `env: { ...process.env, COLMEIA_APP_VERSION: app.getVersion() }` (or add an `env` option to `DaemonClient`).
  Old daemons ignore unknown environment variables.

### 2. `electron/daemonClient.ts:186` — `appVersion` is stale when the app updates but the daemon binary does not

- **Bug:** the mismatch check compares only `running.version` (the binary hash) with `binVersion()`. After an app update
  where `colmeiad` is unchanged (same hash), the running daemon is kept silently and its ping still reports the
  old `appVersion` (set at spawn time). `colmeia --version` falls back to ping (`cli/colmeia.mjs:356`) when no
  package.json is found (the normal case for the staged `cli/` folder), so it prints the wrong app version.
  The same happens when the user picks "Manter daemon atual".
- **Fix:** return `appVersion` from `DaemonClient.ping()`. Then either (a) count `running.appVersion && running.appVersion !== app.getVersion()`
  as a mismatch, or (b) add a cheap daemon command (e.g. `app.version` set) that `attach()` calls to update
  `s.cfg.AppVersion` without a restart. (b) is better because it doesn't kill live cards.
