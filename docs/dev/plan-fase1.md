# Phase 1 — Go daemon `colmeiad` (whole backend)

Decisions (user, 2026-11-13): whole backend → Go; Electron main = thin bridge; behind flag `COLMEIA_DAEMON=1` until validated, then tmux path removed; Linux only (no ConPTY/named pipe yet). No commit/push.
Reference impl = current TS: `electron/*.ts`, `cli/colmeia.mjs`. Port behavior 1:1 unless stated. Phase 0 semantics: `docs/dev/plan-fase0.md`.

## Layout (Go module `colmeia/daemon`, Go 1.27, dir `daemon/`)
```
daemon/go.mod
daemon/cmd/colmeiad/main.go        GoSvc   flags, startup, signal, PR_SET_CHILD_SUBREAPER
daemon/internal/term/              GoTerm  PTY sessions, vt emulator, ring/replay, idle, history on disk
daemon/internal/store/             GoSvc   workspaces, presets, state.json, wsDisk, corrupt/.bak (port store.ts, wsDisk.ts)
daemon/internal/notes/             GoSvc   notesStore.ts + noteWatch.ts (fsnotify)
daemon/internal/links/             GoSvc   links.ts + linkEffects.ts (pipes, note ctx msg, edgeLastSeen)
daemon/internal/auth/              GoSvc   hmac token + peer creds
daemon/internal/api/               GoSvc   socket server, all CLI cmds + ui.* cmds, events
daemon/internal/usage/             GoSvc   usage.ts (claude json + opencode sqlite via modernc.org/sqlite) — last
electron/daemonClient.ts           Bridge  socket client, spawn daemon, rid mux, event stream
electron/main.ts                   Bridge  if COLMEIA_DAEMON=1 → IPC handlers forward to daemon; else old path untouched
```
Deps allowed: `github.com/creack/pty`, vt emulator (`github.com/charmbracelet/x/vt` or `github.com/hinshun/vt10x`, GoTerm picks), `github.com/fsnotify/fsnotify`, `modernc.org/sqlite`, `golang.org/x/sys/unix`. Nothing else w/o asking.
Renderer (`src/*`) + `electron/preload.ts` + `cli/colmeia.mjs` unchanged (preload API is the contract). Exception: CLI may need nothing; if it does → ask manager.

## Contract A — `term` package (GoTerm owns; GoSvc consumes). Write `daemon/internal/term/api.go` FIRST.
```go
type SpawnReq struct { ID, Preset, Title, Cwd, Command string; Args []string; Env map[string]string; Cols, Rows int }
type Info struct { ID, Preset, Title, Cwd string; Alive bool; ExitCode *int; Pid int; Persistent bool }
type Event struct { Kind string /*"data"|"exit"|"title"*/; ID string; Data []byte; Code int }
type Manager interface {
  Ensure(r SpawnReq) (replay string, info Info, err error) // exists → replay; else spawn
  Restart(id string) error
  Write(id string, data []byte) error
  Send(id, text string, enter bool) bool          // bracketed paste rules as ptyManager.send
  Key(id, key string) error                        // enter|esc|ctrl-c|up|down|tab...
  Resize(id string, cols, rows int)               // debounce not needed (renderer debounces)
  SetMeta(id, title string)
  Kill(id string) error                            // process + history file
  Detach(id string)                                // no-op for process; flush history
  FlushAll()                                       // sync history flush (shutdown)
  Prune(keep []string)                             // delete histories not in keep
  List() []Info; Get(id string) (Info, bool)
  Read(id string, lines int) (string, error)       // rendered screen + scrollback, trailing blank trimmed (like tmux capture-pane)
  WaitIdle(id string, idle, timeout time.Duration) (idleOK bool, waited time.Duration)
  ShellPids() map[int]string                       // child pid → card id (for peer-cred auth)
  Subscribe() (<-chan Event, func())              // fan-out; slow subscriber must not block PTY reads (drop+resync)
}
func New(opts Options) Manager // Options{HistoryDir string; HistoryCap int (512KB); Ring int}
```
- Persistence = daemon process lives on after UI quits. Processes die only on Kill or daemon exit. History file `~/.config/colmeia/history/<id>.log` (same format/cap, atomic write, throttle ≤1/10s) → on daemon restart, Ensure of unknown id with history → replay old history + "[sessão anterior encerrada]" line + new process.
- Replay to UI on Ensure: raw ring (≤512KB, cut at safe boundary) OR emulator-serialized screen — pick what renders cleanly for claude/opencode TUIs in xterm.js; document choice.
- Env per card: `COLMEIA_CARD_ID`, `COLMEIA_TOKEN`, `COLMEIA_SOCK` (GoSvc fills `Env`); term just applies.
- Tests (≤3): spawn `sh`, send `echo hi`, Read contains hi; WaitIdle; Kill removes history.

## Contract B — socket protocol (GoSvc server; Bridge client)
Socket `$XDG_RUNTIME_DIR/colmeia.sock`, 0600, JSON per line. Request `{id, cmd, args}` → `{id, ok, result|error}` (identical to TS apiServer). All existing CLI cmds keep names/args/results/error texts.
Auth (`caller`):
1. Peer creds (SO_PEERCRED): uid ≠ daemon uid → reject. Walk `/proc/<pid>/stat` ppid chain: hits pid in `ShellPids()` → caller = that card (ignore args.from/token). Hits daemon pid w/o card (orphan reparented via subreaper) → reject.
2. Else (not descendant): args.from+token → verify hmac (as auth.ts); none → user.
UI cmds (only for conn that did `ui.attach`; `ui.attach` args `{secret}` = contents of `~/.config/colmeia/secret`, and peer must NOT be daemon descendant):
- `ui.attach` → result `{socketPath, presetsPath, home}`; after it daemon pushes events on this conn: `{ev:"pty.data",id,data}` (data utf8 string), `{ev:"pty.exit",id,code}`, `{ev:"canvas",event:<CanvasEvent as TS>}`, `{ev:"note-changed",...}` (as noteWatch), `{ev:"ui.req",rid,op:"screenshot",file}`.
- `ui.reply {rid, ok, result|error}` (answer to ui.req).
- Mirror of every preload IPC, args positional→named: `pty.ensure{req}`, `pty.restart{id}`, `pty.write{id,data}`, `pty.resize{id,cols,rows}`, `pty.kill{id}`, `pty.detachAll`, `pty.killAll`, `pty.setMeta{id,patch}`, `pty.send{ids,text}`, `pty.list`, `app.newId`, `presets.get`, `presets.save{list}`, `ws.list`, `ws.load{name}`, `ws.save{ws}`, `ws.delete{name}`, `ws.rename{from,to}`, `links.set{edges}`, `notes.list{ws}`, `notes.create{ws,title}`, `notes.update{ws,id,patch}`, `notes.remove{ws,id,deleteFile}`, `notes.read{ws,ref}`, `notes.write{ws,ref,text,baseMtime}`, `notes.root`, `usage.get`, `usage.setSettings{s}`, `app.flush` (sync history flush).
- Multiple UI conns allowed (events fan-out); screenshot req → most recent UI.
- No UI attached: canvas-mutating CLI cmds (spawn/link/unlink/note new/rm/focus/screenshot) still update daemon state + workspace JSON on disk where TS did; screenshot → error "UI não está aberta".
Daemon lifecycle: Electron spawns `colmeiad` detached (setsid, stdio → `~/.config/colmeia/colmeiad.log`) if `ping` fails; daemon writes pidfile, single instance (flock). SIGTERM → FlushAll, kill children, exit. `colmeiad --stop` optional.

## Tasks
| worker | model | files (only these) | task |
|---|---|---|---|
| GoTerm (new) | sonnet | `daemon/internal/term/**` | Contract A |
| GoSvc (=AuditBack) | sonnet | `daemon/go.mod`, `daemon/go.sum`, `daemon/cmd/**`, `daemon/internal/{store,notes,links,auth,api,usage}/**` | Contract B server + port services; until term ready, code against api.go |
| Bridge (=AuditFront) | sonnet | `electron/daemonClient.ts`, `electron/main.ts`, `package.json` scripts (build daemon: `go build -o out/colmeiad ./daemon/cmd/colmeiad`) | flag path; old path intact |
| Review | opus | read-only → `docs/dev/review-fase1.md` | after all done |
| Checks | haiku | none | `go vet ./... && go test ./...`, `npx tsc --noEmit`, `npm run build` |

Done = `go vet`/`go test` ✓, tsc+build ✓, with `COLMEIA_DAEMON=1`: app opens, cards spawn/type, CLI list/send/ask/read/wait work, agent with `unset COLMEIA_TOKEN COLMEIA_CARD_ID` still identified (peer creds), quit app → reopen → cards alive + scrollback.

## Round 2 — fixes from docs/dev/review-fase1.md
Contract changes:
- term: session `total` byte counter. `Ensure` returns replay = ring (also for new spawn) + `Offset` = total at end of replay, atomically. `Event{Kind:"data"}` gets `Offset` = byte offset of chunk start. Internal subscriber never drops data/exit (unbounded queue); external lagging subscriber → `Event{Kind:"resync",ID}`.
- socket: `pty.ensure` result `{replay, offset, info}`; `{ev:"pty.data",id,data,offset}`; new `{ev:"pty.reset",id}` (server sends on resync). `ping` result includes `{version}`.
- Bridge: drop/trim data with end ≤ offset; on pty.reset → re-ensure + send renderer "\x1bc"+replay; reconnect + re-attach on socket loss; spawn daemon with `--docs --cli`; version mismatch → dialog ask user before `--stop`+respawn.
Owners: GoTerm H3(term) H4(term) M3(reap all in term) M4 M7 M8 · GoSvc H1 H2 H3(server) H4(server, drop dropPending) H5 M1 M2(doc only) M5(version) M6(both daemon + electron/apiServer.ts) M9 · Bridge H3(reconnect) H4(trim) M5(args+version) M1(strip COLMEIA_DAEMON/COLMEIA_SOCK not needed). LOW: fix if trivial and in own files.

## Round 3 — validate + cleanup (2026-11-13)
- R3a GoSvc(AuditBack, sonnet): no-UI spawn places cards at 0,0 → place next to caller card / grid offset (as renderer does); confirm L2/L3 fixed. Files: daemon/internal/{store,notes,links,api}/**.
- R3b E2E(AuditFront, sonnet): isolated GUI run `COLMEIA_DAEMON=1`, XDG_CONFIG_HOME + XDG_RUNTIME_DIR = scratch dir (never touch real ~/.config/colmeia or real socket). Check Done criteria (Phase 1). Fix bugs only in electron/daemonClient.ts, electron/main.ts. Report → docs/dev/e2e-fase1.md.
- R3c Review(ReviewF1, opus): read-only review of round-2 code (daemon/** + daemonClient.ts + main.ts flag path) → append "## Round 3" to docs/dev/review-fase1.md (new bugs only, HIGH/MED/LOW).
- R4 (after R3 ✓ + user ok): daemon default, remove tmux path (electron/tmux.ts, legacy pty path). Live tmux cards die on switch → user restarts agents.

## Round 4 — fixes from review-fase1.md "## Round 3"
Contract changes: resync Event carries card ID; `{ev:"pty.data",id,data,offset,len}` len = raw byte count (bridge uses len, not byteLength(data)); bridge offset > known → gap → same as pty.reset; daemon flock retry ≤5s at start.
- GoTerm: R3-H1 (term emit resync w/ ID), L4. Files: daemon/internal/term/**
- GoSvc(AuditBack): R3-H1 server (pump via SubscribeAll, reset w/ id, dropPending(id)), R3-H2 daemon flock retry, R3-M1 len field, R3-M3 (slow cmds async/off UI queue, keep write/resize order), L2, L3, L5. Files: daemon/** except term
- Bridge(AuditFront): R3-H1 gap detect, R3-H2 wait old daemon pid exit before spawn, R3-M1 use len, R3-M2 (clear reqs on canvas removeCard / pty.exit), L1, L6. Files: electron/daemonClient.ts, electron/main.ts
Done: go vet + go test -race ✓, tsc ✓, `npm run build` ✓ (Bridge runs build last).
