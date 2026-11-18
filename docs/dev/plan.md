# Plan: auto-clear workers + notes as memory (2026-11-17)

Goal: manager clears a worker's conversation when done with it; worker memory = workspace notes, read fast.

## §1 P1 CLI/daemon — Operaria-Presets (sonnet)
Files: cli/colmeia.mjs, daemon/internal/api/cmds.go (+ its test file if exists).
- New `colmeia clear <id>`: sends tool-correct reset to card (claude → `/clear`, opencode → `/new`; unknown/shell → error). Daemon knows card tool/preset.
- `colmeia ask <id> "..." --result --clear`: after printing result, run clear on that card.
- Usage text updated.
Done: go test ./... ok, node --check cli ok.

## §2 P1 prompts/docs — Operaria-UI (sonnet)
Files: manager prompt source (claude-manager preset, see store.go:158-183 / spawn.go:116 — whichever file holds the text), docs/WORKER.md, docs/ORCHESTRATOR.md.
- Manager: when finished with a worker (task verified/accepted, no further feedback round) → `colmeia clear <id>`. One-shot tasks (scout, verifier) → `ask --result --clear`. Do NOT clear between feedback rounds of same task. Before clearing, worker saved durable facts to notes (ask it in the task prompt: "save to note X"). Fluxo step 7 + token list item 2.
- Worker: memory = notes, conversation will be cleared. Start: `colmeia note ls`, read only relevant note, use rg on note files, no full dumps. End: if learned something costly/reusable → `colmeia note append` 1-2 telegraphic lines, then RESUMO.
- Keep text short, same style/language as existing.
Done: texts consistent with §1 command names.

## §3 Verificador (haiku): go test ./..., npx tsc --noEmit, node --check cli/colmeia.mjs.
## §4 Revisor (opus): git diff of §1+§2.
