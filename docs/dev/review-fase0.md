# Review Phase 0 — findings

Ranked by severity. Owner: [back] = electron/, cli/; [front] = src/.

1. **med [back]** `electron/apiServer.ts:83` + `electron/links.ts` — main knows links/notes of active ws only. Agents in other ws keep running in tmux after switch, but every CLI call fails ("desatualizado"/"sem link") and their pipes stop.
   → keep card→ws map + per-ws edges in main; resolve notes in caller's ws.

2. **med [back]** `cli/colmeia.mjs:82-88` — card id comes from tmux session name, COLMEIA_TOKEN from env. `new-session -A` ignores `-e` on reattach to existing session → token missing/other card's → every call rejected, agent locked out.
   → CLI reads token via `tmux show-environment -t #S COLMEIA_TOKEN`; main runs `set-environment` on attach.

3. **med [back]** `electron/ptyManager.ts:253` — async history write not awaited. `kill` (removeHistory, :303) can be undone by its late rename; sync `flushHistory` on detach shares same `.tmp` while async write in progress → log corruption.
   → per-card unique tmp + pending-promise chain; kill/detach await it or bump generation to drop stale write.

4. **low-med [front]** `src/App.tsx:342` (rename) / delete flow; related `electron/main.ts:128` — autosave cancelled only before IPC `await`. Save firing during await goes under old name → recreates `<old>.json` (copy with same card ids; deleting it later kills shared tmux sessions). `ws:save` also flips `currentWorkspace` back.
   → block autosave (flag) until rename/delete finishes.

5. **low [back]** `electron/ptyManager.ts:125` — `restart` awaits `kill` with session already deleted. `ensure` in that window spawns duplicate, or `new-session -A` attaches to session being killed.
   → "restarting" marker; spawn under it.

6. **low [back]** `electron/notesStore.ts:117-129` — if `newDir` exists (watcher mkdir creates it), files stay in `oldDir` but index moves → watcher misses edits; existing index for new name overwritten.
   → merge, or refuse rename.

7. **low [back]** `electron/apiServer.ts:257` — `screenshot` has no `need()`, writes to any path a card passes.
   → when called from a card, limit to tmp/cwd.
