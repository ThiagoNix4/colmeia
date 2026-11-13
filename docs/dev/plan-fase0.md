# Phase 0 — fix TS app before Go port

Input: `docs/dev/audit-backend.md`, `docs/dev/audit-frontend.md`. No commit/push. No new Electron-only deps (logic → Go `colmeiad` in Phase 1).
Goal: every UI element has real effect; no data loss; cheap perf wins.

## Edge semantics (user chose a+b+c)
- Edge stored directed: `source → target`. `WorkspaceEdge.mode?: 'control' | 'pipe'` (absent = control).
- **control A→B** (terminal→terminal): A may command B (send/ask/read/key/kill/wait/result/link). B ✗ command A. User (no card token) = free.
- **pipe A→B** (terminal→terminal): control + main forwards A's new `RESUMO:` to B. Trigger: A goes idle and last RESUMO text ≠ last forwarded → `pty.send(B, "[from <A title>] RESUMO: ...")` + enter. Dedupe per edge (hash in memory). Only if B is idle-ish? → no, just send (agent queues input).
- **card↔note** (any direction): card may read+write note (`note read/write/append`). On edge creation (UI or CLI) main sends linked card one line: `Context note linked: "<title>". Read: colmeia note read "<title>"` (only if card is a terminal running an agent; skip shell preset). Agent writes via CLI → note card reloads (B1).
- note↔note ✗ (blocked UI + CLI). Notes never targets of broadcast.
- Old edges: keep source/target as-is (=control). UI offers "inverter direção".

## Contract (shared; AuditBack owns electron/types.ts + preload.ts — do first, then AuditFront consumes)
- `WorkspaceEdge` + `mode?: 'control'|'pipe'`.
- `links.set(edges: {source,target,mode}[])` (replaces `[string,string][]`). links.ts directed: `controls(a,b)`, `noteAccess(card,note)`, `pipes(a)`.
- `pty.detachAll()` IPC (renderer: switch ws). `killAll` only on delete ws.
- `notes.read(ws, ref) → {body, mtime}`; `notes.write(ws, ref, text, baseMtime) → {mtime}`; reject Error `conflict` if disk mtime > baseMtime.
- `CanvasEvent` + `{type:'note-changed', id, mtime}` (main fs.watch on notes dir of current ws, debounced 150ms, ignore own writes).
- `CanvasEvent` `link` gains `mode`; new `{type:'unlink', a, b}` if not present.
- CLI: `colmeia link A B [--pipe]`, `colmeia unlink A B`.

## Tasks
### AuditBack (sonnet) — files: `electron/*`, `cli/*`
1. Permission: `link` requires `controls(from,A)||from==A` and `controls(from,B)`/note access; `spawn --near X` requires need(X) when X≠self; note↔note link rejected.
2. Per-card token: `COLMEIA_TOKEN = hmac(secret, cardId)`, secret in `~/.config/colmeia/secret` (0600, generated once); injected via tmux `new-session -e` + non-tmux env. CLI sends `{from, token}`; server: from w/o valid token → reject; no from+no token = user. (Known limit: agent unsetting env = user; fix Phase 1 via peer creds.)
3. Directed links + pipe forwarding + note-link context message (see semantics).
4. Main loads link pairs from current workspace JSON at startup (before renderer `links:set`).
5. Corrupt workspace JSON → rename to `<name>.json.corrupt-<ts>`, keep `.bak` of last good write; never overwrite non-empty file with empty/failed parse result.
6. Notes index migrate on ws rename (rename index file + vault dir) and delete (remove index; keep .md files → move to `<vault>/Colmeia/_deleted/<ws>-<ts>/`).
7. Note path confined: resolved path must stay inside notes root (`path.relative` check) else Error.
8. notes read/write mtime + fs.watch → `note-changed` event; `pty.detachAll` IPC.
9. Perf: tmux `execFile` async (P5); drop headless xterm when tmux persistent, bracketed-paste via regex (P1); history write async + throttle ≤1/10s, sync only on detach/quit (P2); replay ring/compact (P3).
### AuditFront (sonnet) — files: `src/*`
1. Switch ws → `pty.detachAll()` not killAll (App.tsx ~274/324); killAll only on delete; confirm text updated (F12). try/catch restore on load failure (B3).
2. Edge delete: `deleteKeyCode` Delete/Backspace for edges only (not nodes) + edge context menu: excluir, inverter direção, modo control/pipe. Arrow shows direction; pipe style distinct (dashed/label "RESUMO").
3. Block note↔note connect; `connect` requires both nodes exist (B2); send `links.set` with directed edges + mode.
4. Broadcast only terminals (F4); BroadcastBar zustand selector (no full-store subscribe).
5. NoteNode: use `{body,mtime}`; on `note-changed` reload if !dirty, if dirty show "conflito — recarregar/sobrescrever"; write with baseMtime; status label honest (F10).
6. `scheduleNoteGeometry` reads workspace at fire time (B3b).
7. Perf: edge bee animation only when edge hovered/selected or endpoint card active; debounce terminal resize (fit) ~100ms; buffer data until ready (B4).
### Review (Haiku, shell card "Review")
- after both: `npx tsc --noEmit`, `npm run build`, check vs this plan; report gaps.

## Done
tsc + build clean; ≤3 tests (token reject, link w/o perm rejected, notes conflict); log in note "Em andamento".

## Known limits (Phase 0)
- Cards of a non-active ws (tmux alive after switch, no PTY in main): CLI permission + note checks read that ws's edges/notes from disk. They cannot mutate the canvas (`link/unlink/spawn/note new` → error) and are not commandable from the active ws.
- Pipes only forward for cards with a PTY in main (active ws); detached cards' pipes stay off until Phase 1 (`colmeiad`).
- Agent that unsets env = user (peer creds in Phase 1). Cycle A↔B pipe: echo guarded (`[from ` lines ignored), genuine ping-pong not blocked.
