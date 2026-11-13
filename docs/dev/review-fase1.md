# Review — Phase 1 (Go daemon `colmeiad` + Bridge)

Scope: `daemon/**`, `electron/daemonClient.ts`, daemon path in `electron/main.ts`; reference = `electron/*.ts`.
Read-only. Already verified elsewhere: go vet/test, tsc, build, CLI spawn/ask/read, peer-cred block.
Ranked most severe first. `file:line` → problem → fix.

## HIGH

### H1. Requests on one connection run concurrently → reordering (keystrokes, ws.save/ws.load)
`daemon/internal/api/server.go:209` (`dispatch` spawns a goroutine per request).
- `pty.write` keystrokes/paste chunks from the UI can reach the PTY out of order (TS: sync, in order).
- Workspace switch (`src/App.tsx:309-313`): `ws.save(old)` → `pty.detachAll` → `ws.load(new)` are sent in order but executed in parallel. `ws.save` calls `SaveWorkspace` (fsync, slow) *then* `s.watch.Watch(old)` (`ui.go:191-199`), so it usually finishes after `ws.load` → note watcher stuck on the OLD ws (no `note-changed` for the active one); `curWs/curCwd` can also end up = old → CLI `spawn` cwd, `activeOnly`, `callerWs` wrong.
- Two debounced `ws.save` in flight → older snapshot can be written last → canvas changes lost.
Fix: process each connection's requests in order (single worker per conn). Only cmds that can block for long (`wait`, `screenshot`, maybe `read`) go async. At least: UI conns strictly sequential.

### H2. Cards of inactive workspaces stay in the daemon's session table → wrong ws/permissions
`daemon/internal/api/ui.go:118-122` (`pty.detachAll` = flush only), `daemon/internal/api/cmds.go:118-129` (`callerWs`), `cmds.go:250-264` (`list`).
TS: `detachAll` removed sessions (tmux kept them), so `ptys.has(from)==false` ⇒ caller ws read from disk. Go: after switching ws the old cards are still in `Term.List()`, so:
- agent in inactive ws → `callerWs` = active ws → its real links/notes not found → `sem link` on legit targets;
- `activeOnly` passes → inactive-ws agent can `spawn`/`link`/`note new` into the active canvas;
- `colmeia list`/`sessions` show cards of every workspace; title refs may become ambiguous across workspaces.
Fix: daemon keeps `cardID → ws` (from `ws.save`/`ws.load` cards + `spawn`), `callerWs` uses it; `list`/`resolveCard` filter to active ws (unless `--all`), like TS.

### H3. PTY output dropped silently, no resync
- `daemon/internal/term/manager.go:260-268`: subscriber chan (1024) full → event dropped (data **and** `exit`). xterm stays corrupted (lost escape seqs), UI never learns exit; Piper misses idle reset.
- `daemon/internal/api/server.go:145-150`: UI conn backlog 4096 → conn closed. Bridge has no reconnect: `electron/main.ts:301` just shows "Conexão com o daemon perdida", `daemonClient.ts` never re-attaches/re-ensures → app dead until restart.
Fix: per-session monotonic byte offset (see H4). On drop, mark subscriber lagged and emit `{kind:"resync",id}` → server sends `pty.reset{id}` → renderer re-`ensure`s (replay from ring). In term, the only internal subscriber (pump) can use an unbounded queue goroutine instead of drop. Bridge: on close (not quitting) reconnect + `ui.attach` + tell renderer to re-ensure all cards.

### H4. Replay vs live stream race on `pty.ensure` → lost or duplicated bytes
`manager.go:88-111`, `ui.go:104` (`dropPending`), `electron/main.ts` `pty:ensure` handler (`pending.delete`).
- New card: readLoop starts in `spawn` before `Ensure` returns, but `Ensure` returns `saved` (history only), not the ring. First output (shell prompt) buffered in server `pending` is then deleted by `dropPending` → initial prompt missing.
- Existing card: ring snapshot taken under `s.mu`, but readLoop emits after unlock; data emitted between snapshot and `dropPending`/main `pending.delete` is lost; data already pushed to the renderer before the reply is in `pendingChunks` (`TerminalNode.tsx:139-142`) and also in replay → duplicated.
TS was synchronous so the window didn't exist.
Fix: session keeps `total` byte counter; `Ensure` returns `{replay: ring, offset: total}` atomically; data events carry `offset`; renderer/bridge discard chunks with end ≤ offset (or trim the overlap). Remove `dropPending`/`pending.delete` hacks. For new spawn, return `string(s.ring)` (seed + output so far) under lock.

### H5. No UI attached: canvas mutations persisted nowhere
`cmds.go:315-328` (spawn), `421-453` (link/unlink), `477-513` (note new), `550-559` (note rm), `server.go` `emit` (no-op without UI).
Plan Contract B requires daemon state + workspace JSON on disk to be updated. Today: `spawn` without UI → running PTY that never enters any ws JSON → invisible forever (and its history pruned at next daemon start, `server.go:101`); `link/unlink` only in memory → overwritten by next `ws.load`/`links.set`. Also events emitted while Electron is up but window not yet created/loaded are dropped by `send`.
Fix: when `len(uis)==0`, apply the mutation to the ws JSON via store helpers (`AddCard`, `RemoveCard`, `AddEdge`, `RemoveEdge`, note pos) under `store.mu`; or queue canvas events and flush on `ui.attach` after renderer ready. Renderer should also reconcile `pty.list` vs ws cards on load.

## MEDIUM

### M1. Electron launched from inside a card cannot attach
`daemon/internal/api/ui.go:45`. If the app is started from a Colmeia card terminal (user's dev workflow: `npm run dev` inside a card), Electron is a daemon descendant → origin `Card` → `ui.attach` rejected → startup error dialog. `COLMEIA_DAEMON`/`COLMEIA_SOCK` also leak into card env, so a nested dev instance targets the live daemon.
Fix (decide): document; or allow attach when secret OK and peer exe is electron (`/proc/<pid>/exe`) + log it; strip `COLMEIA_DAEMON` from card env; dev instances use their own `COLMEIA_SOCK_PATH`/config dir.

### M2. Peer-cred auth bypassable by non-descendant processes
`daemon/internal/auth/auth.go:162-174`, `server.go` `caller`. A card can run code outside the daemon tree (`systemd-run --user`, `at`, cron, an already-running legacy tmux server whose cards are still alive) → origin `External`; with token unset → role **user** (no restrictions). Same uid can read `~/.config/colmeia/secret` → forge any card token or even `ui.attach` (full pty.write/ws.save). Not a regression vs TS, but the "agent identified via peer creds" guarantee holds only for descendants.
Fix/mitigation: document residual risk; kill legacy tmux sessions when daemon mode is enabled; optionally require that user-role callers have a controlling tty that is not one of the daemon's PTY slaves (`/proc/<pid>/stat` tty_nr vs card pty numbers).

### M3. Subreaper without reaper → zombies accumulate
`daemon/cmd/colmeiad/main.go:178`. Orphaned grandchildren reparent to the daemon but nothing `wait`s them (only `exec.Cmd.Wait` for shells). Long-lived daemon + agents spawning background procs → zombie leak (pid table).
Fix: SIGCHLD loop: `waitid(P_ALL, WNOHANG|WNOWAIT)`; if pid ∉ term-tracked shell pids → `wait4(pid)`. (term exposes tracked pids, or term reaps everything centrally.)

### M4. Card env: ELECTRON_* filter ineffective; env frozen
`manager.go:139-143` starts from `os.Environ()` then appends `r.Env`; `buildSpawn` (`spawn.go:367-371`) filtered `ELECTRON_*`/`CHROME_DESKTOP` but term re-adds them. Daemon env = env of whichever Electron launched it first (frozen for days).
Fix: if `r.Env != nil`, use only `r.Env` (sorted `k=v`).

### M5. Daemon started without `--docs/--cli`; no version check
`electron/daemonClient.ts:128` spawns `colmeiad` with no args; daemon guesses from cwd/exe dir (`main.go:193-198`). Packaged (cwd `/`, bin in `app.asar.unpacked/out`) → `cli` not on card PATH (`colmeia` CLI missing) and Gerente `cat {docs}/ORCHESTRATOR.md` fails. Also after rebuild an old daemon keeps running (ping succeeds) → stale code.
Fix: pass `--docs store.docsDir() --cli store.cliDir()` (unpacked paths). `ping` returns `{version}`; mismatch → `colmeiad --stop` + respawn (cards die — ask user / only in dev).

### M6. Same socket path as legacy apiServer
`main.go:183-188` and `electron/main.ts:16` both use `$XDG_RUNTIME_DIR/colmeia.sock`; each unlinks the other's socket on start (`server.go:86`, `apiServer.ts` `unlinkSync`). Running the app without the flag while the daemon lives (or vice versa) silently reroutes every card's CLI to the other backend (tokens valid in both: same secret).
Fix: before unlinking, try connecting; if alive → refuse/exit with message (both sides). CLI default path can stay.

### M7. Restart ↔ Ensure race (no `settled` guard)
`manager.go:314-336`. Between `delete(m.ss,id)` and `spawn`, a concurrent `Ensure` (renderer remount) spawns another session; Restart's spawn then overwrites the map → first process leaked untracked (never killed; its CLI calls rejected as Orphan). TS had `ptys.settled()`.
Fix: per-id restarting flag/chan; `Ensure` waits on it; or spawn+swap under `m.mu`.

### M8. Detached/removed sessions never die; Prune only deletes history, only at daemon start
`manager.go:451-457`, `manager.go:473`, `server.go:101`. CLI `detach` emits `removeCard` but the process stays in `List()` forever; cards deleted while daemon down etc. accumulate. TS: tmux prune killed sessions not in any ws at every app start.
Fix: `Prune(keep)` also kills live sessions not in keep (except ones created < N s ago); call it on `ui.attach` too.

### M9. A panic in any handler kills the daemon (and every agent)
`server.go:209-216`, also timers (`Piper`, watcher, usage). No `recover`.
Fix: `defer func(){ if r := recover(); r != nil { log; reply error } }()` in the request goroutine and timer callbacks.

## LOW

- L1 `ui.go:22,26`: `c.ui` read without lock (written under `s.mu` in `attach`) → data race (`-race` flags it). Use atomic.Bool.
- L2 `notes.go:21-30,273-297`: `NoteCard` struct drops unknown index fields on load/`Update`/save; TS spread kept arbitrary patch keys. Keep notes as `map[string]any` or add `Extra` passthrough if renderer adds fields.
- L3 `manager.go:239-257`: history written (≤512KB, sync) while holding `s.mu` → blocks readLoop/Write/Read for that card. Copy ring under lock, write outside.
- L4 `links.go:169-171`: Go `\s` is ASCII-only vs JS `\s` (Unicode, e.g. NBSP) in RESUMO regex. Use `[\s\x{00A0}…]` if needed.
- L5 `server.go:261-283` `askUI`: CLI `screenshot` waits 30s on daemon side = client default timeout → client may time out first with generic error. Use 25s.

## Round 3

Scope: round-2 code (`daemon/**`, `electron/daemonClient.ts`, `electron/main.ts` daemon path). New bugs only (round-1 items not repeated unless the fix is broken). Read-only.

### HIGH

#### R3-H1. Resync is a no-op → H3 still unfixed; `exit` can still be lost
- `term/manager.go` `emit`: lagged subscriber gets `Event{Kind:"resync", ID:""}` (no card id).
- `api/server.go` `pumpOne` → `dropPending("")` + `{ev:"pty.reset", id:""}`; `electron/main.ts` `pty.reset` handler → `reqs.get("")` = undefined → nothing re-ensured. xterm stays corrupted exactly as before.
- `server.go` `Start` uses lossy `Term.Subscribe()` for the pump (the only internal subscriber), not `SubscribeAll()` as Round-2 contract says → `exit` events and `piper.OnData` can be dropped.
- Bridge `accept` (`main.ts`) never detects a gap (`offset > have`) → silently concatenates around lost bytes.
Fix: pump uses `SubscribeAll()` (never drops). UI-side lag is already handled by `push`→close→reconnect. Alternatively: track lagged ids per sub and emit one resync per id, and/or treat `id:""` as "all cards" in bridge. Bridge: `offset > have` → `ensure(req, true)`.

#### R3-H2. Version-mismatch restart always fails (flock race) → fatal dialog + app exits
`daemonClient.ts connect()` waits only until the socket file disappears, then `spawnDaemon()`. Old daemon removes the socket first thing on SIGTERM (`srv.Close()` in `cmd/colmeiad/main.go`), then sleeps 500ms+FlushAll while still holding the pidfile flock → new `colmeiad` hits `LOCK_NB` fail, prints "já está rodando", exits 0 → 5s ping loop finds nothing → "colmeiad não respondeu" → `app.exit(1)`. Deterministic on every "Reiniciar daemon".
Fix: in `connect`, after `--stop` wait until pid from `colmeiad.pid` is gone (`process.kill(pid,0)` throws) (≤5s); and/or daemon retries `flock` for ~5s before giving up.

### MEDIUM

#### R3-M1. Offset drift on non-UTF-8 output → bytes dropped after binary/Latin-1 output
`server.go flushPending` sends `string(o.data)`; `json.Marshal` replaces each invalid byte with U+FFFD (3 bytes). Bridge `accept` sets `known = offset + Buffer.byteLength(data)` → `known` overshoots by 2 bytes per invalid byte → next chunk's first k bytes trimmed / whole chunk dropped as "duplicate" (e.g. `cat` a binary, then prompt/output missing). Same for `pty.exit` remainder.
Fix: daemon sends raw `end` (= off+len(raw)) in `pty.data`; bridge uses `known = end` and trims by raw byte count only when it can (or just drops whole chunks with `end ≤ have`, no partial trim).

#### R3-M2. Bridge reconnect resurrects cards killed via CLI
`main.ts`: `reqs` is only cleared on `pty:kill`/`pty:killAll` IPC. CLI `kill`/`detach` → canvas `removeCard` → renderer `st.removeCard` (no IPC) → id stays in `reqs`. On socket loss, reconnect does `ensure(req,true)` for every `reqs` entry → `Term.Ensure` spawns a fresh process for the killed card (history gone, `cardWs` = curWs → shows in `colmeia list`, < 30s so it survives the attach-time prune). Also re-ensures cards of every workspace visited this session.
Fix: on canvas `removeCard` event delete `reqs`/`known` for that id; on reconnect re-ensure only ids present in `pty.list` (alive in daemon) or in the current ws.

#### R3-M3. Slow handlers block keystrokes on the sequential UI connection
`server.go dispatch`: only `wait`/`screenshot` async. On the single UI conn, `pty.write`/`pty.resize` queue behind: `pty.kill` (terminate waits ≤2.5s), `pty.killAll` (N×2.5s, sequential), `pty.detachAll` (sync history write of every session of every ws), `usage.get` (recompute every 15s: scans Claude JSON + opencode sqlite), `ws.save` (fsync), `ui.attach`→`prune` (sequential Kill of stale sessions; many stale → attach > 30s client timeout → fatal on startup).
Fix: keep strict order only where needed (pty.write/resize/send per id, ws.save/ws.load/links.set); run `usage.*`, `pty.kill*`, `notes.*`, `app.flush` async; `prune` in a goroutine after replying; Kill in parallel.

### LOW
- L1 `main.ts` `pty.exit`: exit forwarded immediately while the last data is still in the 8ms `queue` → renderer sees exit before the final output (crash message after "[exit]"). Flush `pending` for that id before `send('pty:exit')`.
- L2 `ui.go ws.rename`: `cardWs` entries keep old name for a non-active ws → agents there get `sem link`/wrong notes until that ws is loaded. Rename map values `from→name`.
- L3 `cmds.go visible`: card absent from `cardWs` = visible. After CLI `detach` + renderer `ws.save`, the detached (still running) card has no entry → still in `colmeia list`/resolvable until next attach prune. Mark detached ids (e.g. `cardWs[id]="\x00detached"`) or kill on detach after flush.
- L4 `manager.go terminate`: on timeout (process ignoring SIGKILL is impossible, but a setsid'd grandchild holding the slave keeps `ptmx.Read` open) readLoop + ptmx fd leak forever; Close `ptmx` after the timeout.
- L5 `server.go pumpOne` gap path: new `chunk` overwrites `s.pending[id]` that may hold an incomplete-UTF-8 tail left by `flushPending` → those bytes lost. Only after a gap; fix with R3-H1.
- L6 `main.ts ensure`: two concurrent ensures of same id (remount + reset) share `ensuring` key; first `finally` deletes the second's held list → chunks accepted with stale `known`. Use a counter/token per id.

## Round 4 check

Verified against code (read-only), not run.

| item | status | note |
|---|---|---|
| R3-H1 resync / lossy pump | ✓ | pump → `SubscribeAll()` (no drops, `exit` safe); term resync carries id (`lagged` map); server `dropPending(id)`+`pty.reset{id}`; bridge `offset > have` → `ensure(req,true)`. |
| R3-H2 restart flock race | ✓ | bridge waits `kill(oldPid,0)` fails before spawn; daemon retries flock 5s. |
| R3-M1 non-UTF-8 offset drift | ✓ | `sendData` adds raw `len`; bridge uses it; partial trim only when `byteLength==len`. |
| R3-M2 reconnect resurrects killed cards | ✓ (mostly) | `reqs` cleared on canvas `removeCard` and `pty.exit`. Still re-ensures cards of other ws visited this session (alive → harmless). |
| R3-M3 slow cmds on UI queue | ✓ | kill/killAll/detachAll/flush/usage/notes async; prune after reply; killAll parallel. See N1, N4. |
| L1 exit before tail | ✓ (bridge) | daemon side can still invert, see N3. |
| L2 rename cardWs | ✓ | |
| L3 detached still visible | ✓ | marker `"\x00detached"`. |
| L4 terminate fd leak | ✓ | `ptmx.Close()` after timeout. |
| L5 tail lost on gap | ✓ | |
| L6 concurrent ensure | ✓ | per-id token; superseded held list is covered by the newer replay. |

### New issues

- **N1 HIGH** `api/ui.go pty.killAll` kills `t.List()` = sessions of **every** workspace (TS killed only the active ws's ptys). Renderer calls it when deleting the active ws (`src/App.tsx:388`) → all agents in all other workspaces die. Now async + parallel, it can also kill cards of the next ws that `loadWorkspace` just ensured. (Present since round 2, missed in R3.) Fix: kill only ids with `cardWs[id]==curWs` (snapshot at dispatch time, before going async).
- **N2 MED** gap detection turns a daemon-side reorder into a full terminal reset: `flushPending` sets `flushT=nil` and broadcasts after releasing `pmu`; the next 8ms timer (or the gap path in `pumpOne`, or `exit`'s sync `flushPending`) can broadcast concurrently → chunk n+1 reaches the bridge before n → `offset > have` → `\x1bc`+512KB replay (screen flash; under heavy output can repeat). Fix: serialize sends (one `sendMu` held across extract+push; `push` is non-blocking).
- **N3 LOW** same race: `exit` → sync `flushPending` finds nothing while a timer goroutine still holds extracted data → `pty.exit` sent before last data; bridge then accepts that data after exit with `known` deleted. Fixed by N2's mutex.
- **N4 LOW** `notes.*` now async per request → two `notes.update`/`notes.write` for the same note can apply out of order (stale position/size, spurious baseMtime conflict). Keep notes.* in-order (they're fast) or serialize per note id.
- **N5 LOW** bridge: `pty.exit` deletes `reqs[id]`; `pty:restart` doesn't re-add it → restarted card is skipped on reconnect re-ensure and any gap on it drops chunks forever (`accept` returns without reset when `reqs` lacks id). Re-set `reqs` in `pty:restart` (keep last req) and, on gap without req, at least `known.delete(id)` + queue.
- **N6 LOW** `cmd/colmeiad/main.go`: old daemon `os.Remove(pidf)` at exit while a new instance may already hold an fd on the same inode waiting on flock → new daemon locks/writes an unlinked file → no pidfile on disk (`--stop` fails, a 3rd instance could lock a fresh file). Don't remove pidf on exit (truncate instead) or re-open+compare inode after locking.
