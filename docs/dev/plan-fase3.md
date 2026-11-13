# Phase 3 (revised) — Electron thin UI + Go daemon only

Decision 2026-11-13: Wails dropped (WebKitGTK canvas/zoom bugs, no RAM gain). Keep Electron as UI; colmeiad (Go) = only backend. Remove all legacy/failed code.
Backup: ~/colmeia-backup-2026-11-13-pre-f3.tgz

## Rules
- Running app = `electron-vite dev` legacy tmux mode. ✗ run `npm run dev`, ✗ `npm install`/`npm prune`, ✗ kill tmux/colmeiad, ✗ touch node_modules. Manager does install at end.
- Only edit files you own. Unsure if code used → `rg` before delete.

## Tasks
### T1 — Electron main (AuditBack, Sonnet) · owns `electron/**`, `package.json`, `electron.vite.config.ts`, `scripts/**`
- daemon path = only path: drop `COLMEIA_DAEMON` flag + legacy branch in `main.ts`
- delete legacy modules no longer imported by daemon path (likely ptyManager, tmux, historyStore, apiServer, store, notesStore, usage, links, noteWatch, linkEffects, auth, wsDisk — verify each w/ rg)
- `types.ts`/`preload.ts`: drop legacy-only items; keep renderer API identical unless dead
- package.json: rm deps node-pty, @electron/rebuild, @xterm/headless, html-to-image (if unused), scripts rebuild/postinstall/desktop:*; electron-builder: targets AppImage + deb, extraResources/asarUnpack `out/colmeiad`, drop node-pty unpack
- scripts/install.sh + restart-colmeia.sh: no tmux; rm `tmux -L colmeia` usage
- done: `npx tsc --noEmit` + `npx electron-vite build` ok

### T2 — Renderer (AuditFront, Sonnet) · owns `src/**`
- delete `src/bridge/` (Wails shim) + its import in `src/main.tsx`
- rm Wails/tmux-only branches/comments in src
- done: tsc ok (only src errors are yours)

### T3 — Daemon (GoTerm, Sonnet) · owns `daemon/**`
- rm code existing only for Wails client or tmux migration/compat; keep socket protocol used by CLI + Electron bridge
- done: `go vet ./... && go build ./...` ok

### T4 — Manager: rm `desktop/`, `wails-spike/`, `vite.desktop.config.ts`, docs/dev phase2 (plan-fase2*, spike-fase2, review-fase2)

### T5 — Docs (Haiku, after T1-3) · owns `README.md`, `docs/agents/**`, `docs/ORCHESTRATOR.md`, `docs/WORKER.md`, vault `Projetos/Colmeia - Arquitetura.md`
- remove tmux/node-pty/Wails/COLMEIA_DAEMON mentions; arch = Electron UI → preload IPC → main bridge → colmeiad socket ← CLI

### T6 — Review (ReviewF1, Opus) diff vs backup → docs/dev/review-fase3.md; fixes back to owners
### T7 — Manager: npm install, build, `npm run dist` (AppImage+deb), user restart → tmux cards die, app on daemon
