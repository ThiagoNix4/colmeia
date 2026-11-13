# Review Phase 3 (T6) — diff ~/colmeia-pre-f3 → ~/colmeia

Scope: electron/, src/, cli/, daemon/, package.json, scripts/. Code not edited.
Totals: H 4 · M 3 · L 4

Checked ✓:
- preload ↔ main IPC: all 31 invoke/send channels have ipcMain handlers; 5 push channels (pty:data, pty:exit, canvas:event, usage:changed, app:flush) emitted by main.
- legacy modules (ptyManager, tmux, store, apiServer, notesStore, usage, links, noteWatch, linkEffects, auth, historyStore, wsDisk) removed; no dangling imports; no `tmux|node-pty|wails|COLMEIA_DAEMON` left in electron/src/cli/daemon/scripts.
- src: `bridge/` + import removed (src/main.tsx:1).
- CLI auth w/o tmux: daemon sets `COLMEIA_CARD_ID`, `COLMEIA_TOKEN`, `COLMEIA_SOCK`, `PATH=cliDir:…` per card (daemon/internal/api/spawn.go:99-101); CLI reads env (cli/colmeia.mjs:72-76). Removing tmux override is correct.
- startup: initial connect failure → error box + exit (electron/main.ts:90-94); window only after `ready` (electron/main.ts:338-345).
- reconnect: onClose → 20×1s retry → re-attach → re-ensure all cards with reset (electron/main.ts:236-265); suppressed during quit and during connect() (electron/daemonClient.ts:69).
- daemon bin path: `app.asar/out/colmeiad` → `app.asar.unpacked/…` (electron/main.ts:62-65) + `asarUnpack: out/colmeiad` (package.json:29-31) ✓.

## H

H1 — package.json:29-31 · docs/ and cli/ not unpacked from asar.
`files` puts cli/** and docs/** inside app.asar, but `asarUnpack` lists only `out/colmeiad`. main passes `unpacked(resDir('docs'|'cli'))` = `…/app.asar.unpacked/{docs,cli}` (electron/main.ts:81), which won't exist when packaged. Go daemon can't read asar → packaged build: cards' PATH has no `colmeia` CLI, statusline (`node <cliDir>/colmeia-statusline.mjs`, spawn.go:21) broken, docs missing. Pre-existing, but now the only path.
Fix (T1): `"asarUnpack": ["out/colmeiad", "cli/**", "docs/**"]` (or move them to `extraResources` and resolve via `process.resourcesPath`).

H2 — scripts/restart-colmeia.sh:37 · deletes socket of a live daemon.
After `pkill` of Electron the daemon (and all cards) keep running, but `rm -f …/colmeia.sock` orphans it. New app: ping fails → spawns 2nd colmeiad → it can't take the pidfile flock (old one holds it) → "colmeiad não respondeu" → fatal; or, if it does start, old cards are unreachable. Also comment line 29 ("fecha todos os cards") is now false.
Fix (T1): drop the `rm -f` line; update comment (daemon/cards survive restart; daemon version mismatch prompts in-app).

H3 — package.json:4-5,32-35 · `deb` target will fail in T7: `author` has no email.
electron-builder deb requires a maintainer ("Please specify author 'email' in the application package.json"). `npm run dist` aborts before producing the .deb (also add `homepage` to avoid warning).
Fix (T1): `"author": {"name": "Thiago", "email": "…"}` or `"linux": {"maintainer": "…"}`.

H4 — electron/main.ts:62,81 + electron/daemonClient.ts:141-147 · AppImage + detached daemon run from the FUSE mount.
In AppImage, `app.getAppPath()` is under `/tmp/.mount_XXXX/`. colmeiad is spawned detached from there with `--cli/--docs` pointing inside the mount, and outlives the app by design. When the app closes the mount goes away → daemon executable pages/CLI dir/docs vanish (SIGBUS or ENOENT); on reopen the daemon still has the old dead paths (and new mount path ≠ → `binVersion` reads different file, may prompt restart every launch). deb (/opt/Colmeia) is fine.
Fix (T1): when `process.env.APPIMAGE` is set, copy `colmeiad`, `cli/`, `docs/` to `~/.local/share/colmeia/<version>/` (atomic tmp+rename) and spawn/pass those paths; or ship deb only.

## M

M1 — electron/main.ts:358-383 · window close (X) skips history flush and may lose last ws.save.
X → `window-all-closed` → `app.quit()` → `before-quit` with `win` already destroyed → no `app:flush`, no `daemonFlush()`. Renderer's `beforeunload` save (src/App.tsx:215) goes via `fire()` (async on `ready.then`) and the process may exit before the socket write drains. Only menu/signal quits get the full flush.
Fix (T1): in `before-quit`, if `!flushed` and window gone → `preventDefault()`, `await daemonFlush()` (same socket, so it serializes after ws.save), then `flushed = true; app.quit()`.

M2 — cli/colmeia:2 + daemon/internal/api/spawn.go:21 · CLI/statusline need system `node`.
Packaged AppImage/deb no longer ships/needs node-pty but CLI still does `exec node …`. On a machine without node, every `colmeia …` inside cards fails. User has node; packaging claim is weaker than it looks.
Fix (T1/T3): fallback `ELECTRON_RUN_AS_NODE=1 <app exe>` when `node` missing (daemon can pass app exe path as env), or document node as a dependency (deb `depends`).

M3 — electron/daemonClient.ts:141-147 + electron/main.ts:81 · daemon spawn args are fixed by whoever starts it first.
Dev instance (`electron-vite dev`, paths ~/colmeia/{cli,docs}) and packaged app share the same socket/daemon; whichever started it wins `--cli/--docs`. Same-version binaries skip the mismatch prompt, so a packaged app can run against a daemon pointing at the dev tree (or vice versa, dead AppImage paths — see H4).
Fix (T3/T1): daemon reports cli/docs dirs in `ping`/`ui.attach`; Electron warns or restarts on mismatch; or socket per install.

## L

L1 — electron/types.ts:34-36 · stale comment "Opcional enquanto o ptyManager é atualizado… (PLAN-PERSISTENCIA §3)"; ptyManager is gone. Fix: drop the line / make `persistent` required if daemon always sends it.
L2 — electron/main.ts:21,52-53,108,391 · comments still contrast with old path ("daemon:", "mesmo throttle do caminho antigo"); `daemonFlush` nullable no longer needed (always set). Cosmetic.
L3 — package.json:27 · `scripts/**` packaged (install.sh, restart, test-fase0, dev launcher) — useless/misleading inside AppImage/deb. Drop from `files`.
L4 — package.json:16,32-35 · target set twice (`--linux AppImage deb` in script + `linux.target`). Keep one (config) → `electron-builder --linux`.

## Recheck (2026-11-13)

Checks: `tsc --noEmit` ✓ · `go vet` + `go build` ✓ · `node --check cli/colmeia.mjs` ✓

- H1 ✓ asarUnpack += `cli/**`, `docs/**` (package.json:32-36)
- H2 ✓ `rm -f colmeia.sock` removed; comment updated (scripts/restart-colmeia.sh:28-35)
- H3 ✓ author email + homepage + `linux.maintainer` (package.json:5-9,40). Email is placeholder `noreply@colmeia.local`, OK for build.
- H4 ✓ `stageAppImage` copies bin/cli/docs → `~/.local/share/colmeia/<version>/` (tmp+rename), used when `$APPIMAGE` (electron/main.ts:60-96)
- M1 ✓ X-close path: `before-quit` preventDefault → `daemonFlush()` → quit (electron/main.ts:412-423)
- M2 ~ deb `depends: nodejs` ✓; AppImage still needs system node (cli/colmeia:2). Accepted residual → doc it.
- M3 ✓ daemon `ping` returns cliDir/docsDir (daemon/internal/api/cmds.go:289); client prompts restart on dir mismatch (electron/daemonClient.ts:175-181)
- L1 ✓ stale comment gone, `persistent` required (electron/types.ts:33-34)
- L2 ✓ comments cleaned; `daemonFlush` non-nullable (electron/main.ts:21-22,139)
- L3 ✓ `scripts/**` out of `files`
- L4 ✓ `dist` → `electron-builder --linux`, targets only in config

New:
- L5 — electron/main.ts:65 · staged dir keyed by version only: rebuilt AppImage with same `version` reuses stale colmeiad/cli/docs. Fix: key by version + bin hash/mtime, or bump version per dist.
- L6 — electron/daemonClient.ts:175-181 · dev and packaged installs now differ in cli/docs → every switch between them prompts restart (kills cards if accepted). Expected; note for user.

Verdict: all H/M closed (M2 residual AppImage-only). Ready for T7.
