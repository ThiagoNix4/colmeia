# AUDIT — frontend (src/**)
Scope: src/**; contracts checked vs electron/preload.ts, types.ts, apiServer.ts, main.ts.
Legend: sev H/M/L · effort S/M/L. Read-only audit, no code edited.

## 1. PERF
| # | sev | eff | where | problem → fix |
|---|---|---|---|---|
| P1 | M | S | src/nodes/TerminalNode.tsx:554-557 | ResizeObserver → `pty.resize` IPC every rAF while user drags NodeResizer → SIGWINCH storm into tmux/agent (TUI redraws). → debounce 120ms trailing, or resize only on `onResizeEnd`. |
| P2 | M | M | TerminalNode.tsx:472, App.tsx:353 | WebGL opt-in only (`colmeia.webgl`) → default DOM renderer: N cards × scrollback 5000 = heavy layout/paint. All nodes always mounted (no `onlyRenderVisibleElements`). → lower scrollback (2000; tmux holds history anyway), pause `term.write` for cards off-viewport/zoom<0.3 (buffer chunks, flush on visible), or render a static placeholder at low zoom. |
| P3 | M | S | src/edges/SmartEdge.tsx:129-131 + styles.css:153 | Per edge: SMIL `animateMotion` repeatCount=indefinite + dashed path. Runs forever even idle → constant SVG repaint, CPU grows with E. `animated:true` (store.ts:324, App.tsx:124) also adds RF dash animation class (redundant, ignored by CSS override but still adds class). → drop `animated`, run bee only on hover/selected edge or when target card is active; CSS `@media` reduced-motion already hides it. |
| P4 | M | S | src/components/BroadcastBar.tsx:21-22 | Subscribes to whole `nodes` + `edges` → re-renders + `useMemo` rerun on every drag frame (position change = new nodes array). → selector returning only selected ids joined (`s.nodes.filter(sel).map(id).join()` with shallow eq), and edges only when target==='linked'. |
| P5 | L | S | src/App.tsx:33-34 | App subscribes to `nodes`/`edges`: re-render per drag frame (RF controlled mode needs it; MiniMap also). Acceptable; note only. |
| P6 | L | S | src/store.ts:298-304 | `onNodesChange` loops all nodes and `prev.find` per note → O(n²) per frame. → iterate `changes` (only `position`/`dimensions` types for note ids). |
| P7 | L | S | App.tsx:147-153 | Autosave fires on every nodes change incl. `selected`/`dimensions` after load → extra `ws:save` of identical JSON 1s after open/click. → compare serialized geometry sig before scheduling; skip on select-only. |
| P8 | L | S | App.tsx:156-163 | `links:set` sent whenever `edges` ref changes (applyEdgeChanges returns new array even for select/measure changes). → dedupe by sorted pair string. |
| P9 | L | S | edges/slots.ts:447-466 | `getSlots` selector runs per edge per store tick; each call does `signature()` O(V) → O(E·V)/frame. Fine ≤20 edges. → compute once per `nodeLookup` change via single store subscription. |
| P10 | L | S | NoteNode.tsx:63-68 | `setText` per keystroke re-renders whole node (textarea controlled) — ok; but `setSave('dirty')` each key. Cheap. Ignore. |
| P11 | L | M | terminalBus.ts | One global listener, fine. But no backpressure: `term.write(chunk)` ignores callback; a `yes`/log flood from agent can freeze renderer. → count pending writes, pause via main (`pty:pause/resume`) when >~1MB queued. Main already batches (main.ts:49). |
| P12 | L | S | UsageMeter.tsx:564 | `fmtReset` computed at render, never ticks → stale countdown. Add 60s tick only while popup open. No polling otherwise (good: push via `usage:changed`). |

IPC chatter otherwise OK: pty:write/resize are `send`, ws:save debounced 1s, notes geometry debounced 350ms, usage push-based.

## 2. FAKE / DRIFT
| # | sev | eff | where | what |
|---|---|---|---|---|
| F1 | H | M | store.ts:309 + App.tsx:374 (`deleteKeyCode={null}`) | **No way to delete a link in the UI.** Edge select works but Delete/Backspace disabled, no edge context menu/button. Only `colmeia unlink`. A link = permission, so user cannot revoke it visually. → enable delete for edges only (`deleteKeyCode` + `onBeforeDelete` that rejects nodes), or add ✕ on selected edge midpoint. |
| F2 | M | S | SmartEdge.tsx bee animation | Bee "flowing" implies data flow A→B. Real effect = symmetric CLI permission only (links.ts `linked`), no data routed, no direction. Misleading. → tooltip/label "permissão"; or show direction only if semantics ever become directed. |
| F3 | M | S | store.ts:309-329, Ports.tsx | Link note↔note and note↔note allowed; notes cannot call CLI, so edge has zero effect. Also terminal↔terminal edge has effect only for CLI `from` calls; UI Broadcast ignores links except 'linked' target. → block note↔note in `connect`/`isValidConnection`. |
| F4 | M | S | BroadcastBar.tsx:27-36 | Targets include note ids (`all` = every node, `selected` too, `linked` neighbors may be notes). `pty.send` to non-pty id → silently dropped but UI says "N alvo(s)" and flash uses `sent.length` (ok) — count shown before send is wrong. → filter `type==='terminal'`. |
| F5 | M | S | UsageMeter.tsx:589,668-677 | Daily-limit input starts `''` and is never hydrated from saved `opencodeDailyLimit` → shows "sem limite" even when a limit is set; blur with empty clears the limit unintentionally. → init from `report` (needs main to return settings in `UsageProvider`) . |
| F6 | M | M | main.ts:118-122 vs notesStore.ts | **Workspace rename/delete does not migrate/remove the notes index** (notes keyed by workspace name; no rename/delete in notesStore). After rename, `notes.list(newName)` → empty: notes vanish from canvas (files remain in vault). Delete leaves orphan index. UI (App.tsx:297-330) assumes it works. → add `notes.renameWorkspace/deleteWorkspace` in main handlers. (outside src, contract drift) |
| F7 | L | S | AgentsMenu.tsx:42-50 | `managerPreview` duplicates `managerCommand` in electron/store.ts → drift. Preview-only. → expose command builder via IPC (`presets:preview`) or share a module. |
| F8 | L | S | AgentsMenu.tsx:155 | `keepShell` for configurable Manager forced `true`, and `keepShell` in menu shown only for user agents; no check that main honors it for tool agents. Verify ptyManager. |
| F9 | L | S | TerminalNode.tsx:643,670 | Deleted preset → card shows raw preset id, grey; restart spawns with missing preset. → fall back to shell + warn. |
| F10 | L | S | NoteNode.tsx:44-47 | Status "salvo" set after write even if user typed meanwhile (dirty reset before write, label overwritten) → label lies. Track write seq. |
| F11 | L | S | Toolbar.tsx:224 | Delete message says "cards" but notes index (F6) isn't deleted either; text/behavior drift. |
| F12 | L | S | App.tsx:290/Toolbar.tsx:290 | Switch-workspace confirm says terminals end (`killAll`); tmux sessions of persistent cards presumably killed too — contradicts PLAN-PERSISTENCIA "sobrevive". Verify `killAll` semantics (detach vs kill). |

Working & real (verified): link→permission (`need()` in apiServer.ts:57-67, `links:set`), `list.linked`, `note ls` filtered by link, broadcast 'linked', workspace/cards/edges/viewport persistence (toWorkspace), note geometry persisted via notes index, preset save, title rename → `pty:setMeta`.

## 3. BUGS
| # | sev | eff | where | bug → fix |
|---|---|---|---|---|
| B1 | H | M | NoteNode.tsx:23-49 | **Lost update:** note read once on mount; no refresh when agent (CLI `note write/append`) or Obsidian edits the .md. Next keystroke autosave (600ms) overwrites the agent's write with stale text. Agents writing notes is the core feature. → main emits `note:changed(id)` (fs.watch/mtime) → reload if !dirty; on dirty conflict, compare mtime before write / merge prompt. |
| B2 | M | S | App.tsx:116 vs 205 | `link` event handled by `connect` without checking both nodes exist → dangling edge in store → `links:set` sends it, saved to ws (filtered only on load). Order addCard→link ok, but CLI `link` to a note/card from another workspace dangles. → in `connect` require both ids in `nodes`. |
| B3 | M | S | App.tsx:268-278 | `switchWorkspace`: `set({ready:false,nodes:[]})` triggers autosave subscribe → timer (1s) later may fire after load of the new ws; ok, but if load fails (reject) `ready` stays false forever, UI blank, no error. → try/catch, restore previous. Same in deleteWorkspace. |
| B3b | M | S | App.tsx:301-306 | rename: save → rename → `set({workspace:name})`; main `currentWorkspace` updated, but renderer pending note-timers (`scheduleNoteGeometry`) captured old `workspace` name (store.ts:264) → update to old name → note index error/orphan (silently `.catch`). → read workspace at fire time. |
| B4 | M | S | TerminalNode.tsx:536-539 | Data arriving before `ready=true` is dropped (`ready && term.write`). Chunks emitted by main between replay snapshot and renderer `then` run are lost/duplicated. Mostly masked by IPC ordering, but a gap is possible on slow font load (`fontReady`). → buffer while !ready, flush after replay. |
| B5 | L | S | TerminalNode.tsx:613-617 | restart: status stays prior until invoke resolves; on failure `restart` throws unhandled. Also doesn't set 'starting'. |
| B6 | L | S | TerminalNode.tsx:608-611 | close = `pty.kill` + remove; if user closes then Ctrl+Z nothing; no confirm for running agent (costly). Add confirm if alive. |
| B7 | L | S | App.tsx:253-266 | `window.alert` for errors blocks Electron UI thread; use toast. |
| B8 | L | S | App.tsx:45-74 | `freeSpot` centers on `window.innerWidth/2` incl. toolbar/broadcast offsets, ignores zoom of card size (CARD_W flow units fine). Minor offset. |
| B9 | L | S | Toolbar.tsx:290 | select `onChange` cancel keeps controlled value ok; but `count` counts only terminals; a workspace with only notes switches without confirm but pending note write handled by unmount cleanup — ok. |
| B10 | L | S | AgentsMenu.tsx:152 | `agent-${Date.now()}` id collision impossible in practice; fine. `persist` error regex duplicates `ipcError` (App.tsx:27) → dedupe helper. |
| B11 | L | S | UsageMeter.tsx:588 | `pick` id not validated vs providers (falls back to first, ok). |

## 4. PORT TO RUST (Tauri)
Easy:
- store.ts (zustand), Toolbar/AgentsMenu/UsageMeter/BroadcastBar/NoteNode UI: pure React, run unchanged in Tauri webview.
- `window.canvas.*` surface is one 70-line preload → replace with a `tauri-bridge.ts` (`invoke` + `listen`); `invoke` errors are plain strings (drop `ipcError` regex).
- ws/notes/presets/usage/links: simple JSON/fs + in-memory set → trivial in Rust (serde, atomic tmp+rename, `notify` for fs.watch). `links.rs` ~30 lines.
- Unix socket API server: tokio `UnixListener` + JSON lines; CLI unchanged.
Medium:
- Events: `canvas:event` → `app.emit`; `onFlush/flushed` → `on_window_event(CloseRequested)` + prevent_close + ack.
- `pty:send` multi-target + `wait` (prompt-idle detection) → needs port of ptyManager logic.
- Usage meter: parsing claude/opencode local files → serde, ok; `fs.watch` → `notify`.
Hard:
- node-pty + tmux layer: `portable-pty` crate works, but replay buffer (history store 1MB tail), tmux attach/persist, resize semantics, backpressure (P11) must be reimplemented; this is the bulk of risk.
- xterm.js stays JS (webview). WebGL addon: WebKitGTK (Linux Tauri) WebGL is flaky/slow vs Chromium → DOM renderer perf (P2) gets worse; test early. Fonts (`document.fonts` CanvasTerm) ok.
- Tauri on Linux = WebKitGTK: SMIL `animateMotion`, `color-mix()`, `drop-shadow` support/perf differ → P3 fix becomes more important; re-verify CSS.
- Screenshot command (main.ts:158 `webContents.capturePage`) has no Tauri equivalent → needs OS-level capture or html2canvas.
- `dialog:pickDir` → tauri-plugin-dialog (easy).

## Top 5 to do first
1. B1 note lost-update (H/M) — core agent↔note flow.
2. F1 edge delete in UI (H/M) — link is permission, must be revocable.
3. F6 notes index on workspace rename/delete (M/M) — data appears lost.
4. P3 + P1 + P4 cheap perf wins (M/S).
5. F3/F4 block note↔note links and filter broadcast to terminals (M/S).
