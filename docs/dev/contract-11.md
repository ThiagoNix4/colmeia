# Contract §11 (daemon/cli -> UI)

## Presets (11A)
- Built-ins now: `shell` (Terminal), `claude-manager` (Gerente), `claude-reviewer` (Revisor, claude opus, worker rules). claude-haiku/sonnet/opus removed (old cards with those ids still spawn).
- Preset JSON gains `args?: string` (extra CLI args appended to the command) and `edited?: boolean` (built-in has user overrides).
  `Builtin: true` + `configurable: true` on BOTH claude-manager and claude-reviewer = editable in the Agentes tab (label, color, tool, model, args).
- `presets.save` (existing UI cmd): send the full list as before. Entries for claude-manager / claude-reviewer are read as overrides
  (`label,color,tool,model,args`). Send `reset: true` on such an entry to restore its default (drops overrides). Result/event `{type:"presets", list}` unchanged.
- Manager/worker rules (ORCHESTRATOR/WORKER injection, --disallowedTools Agent) are always re-applied by the daemon.
- Reviewer label: "Revisor", or "Revisor (padrão)" if the user already has a user preset labeled "Revisor".
- CLI: `colmeia preset reset <id|title>` restores a built-in.

## Layout (11C)
- Sizes: card 640x400, note 420x320, gap 40. Manager at (0,0); workers in a grid (3 cols) right of it; notes in their own column left of it.
- `addCard` event: `card` now carries optional `x`,`y` (and `width`,`height`) = suggested top-left. UI: use them if the spot is free, else `freeSpot`.
- `addNote` event: `note.x`,`note.y`,`note.width`,`note.height` set by the daemon (same rule).
- NEW canvas event (via the normal `{ev:"canvas", event}` channel), emitted by `colmeia tidy`:
  `{type:"layout", cards:[{id,x,y,width,height}], notes:[{id,x,y,width,height}]}` — UI sets positions/sizes of those nodes (ignore unknown ids) and persists as usual.
- CLI `colmeia tidy` -> daemon cmd `tidy` (no args) -> returns `{cards:N, notes:M}`.
