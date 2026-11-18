# Plan: Colmeia improvements (2026-11-17)
Backlog: docs/dev/backlog.md. Scout maps: docs/dev/scout-a-result.md, scout-b-result.md (file:line hints).
Rules: no commit, no push, no npm install, no build (verifier does it). Only touch your files. Keep code style. Go: gofmt.
Out of scope now: 12 (admin), 14 --raw, 15 (done).

## §0 Contract (P0, shared CLI <-> daemon)
- wait: request `{ids: string[], mode: "all"|"any", idle, timeout}` (single `id` still accepted).
  response `{results: [{id, title, status: "ok"|"error"|"exited"|"timeout", seconds, exitCode?: number, lastLine?: string}]}`.
  status "error" = card back to prompt and last output lines match error markers (CommandNotFoundException, ParserError, "ERROR", "Error:", "error:", "not recognized", "não é reconhecido"). lastLine = last non-empty non-prompt line, filled when status != ok.
  CLI prints one line per id: `<title>: ok após Ns` | `<title>: ERRO após Ns — <lastLine>` | `<title>: saiu (code N)` | `<title>: timeout`. Exit code 1 if any error/exited!=0/timeout.
- spawn: request gets `model?: string`. CLI flag `--model <m>`. opencode preset → existing OPENCODE_CONFIG model path; claude presets → `--model <m>` arg.

## §1 Operaria-CLI — files: cli/colmeia.mjs, docs/agents/referencia-cli.md
- 3: no shell:true in spawn/execFile (run .cmd via cmd.exe /d /s /c with proper quoting, or resolve real exe); no DEP0190 in output.
- 19/21: stdin `-` read as Buffers → concat → utf8, strip leading U+FEFF (send, note append, write, all stdin users).
- 10: `result` prints only RESUMO block (stop at blank line / known TUI footer like `Build ·`, `▣`, prompt line; max 8 lines).
- 5: costOf: opencode-go/*-free & *free → free; opencode/big-pickle → free; `colmeia agents set <model> free|paid` persists override (same cache dir as agents cache).
- 1: `agents` adds column with exe path or ✗ when tool not found on PATH (also check %APPDATA%\npm, ~/.opencode/bin).
- 4/13: `wait <id...> [--all|--any]` per §0.
- 2/11: `spawn <preset> --model <m>`; aliases: `colmeia spawn claude-haiku|claude-sonnet|claude-opus` work (daemon provides presets).
- Update docs/agents/referencia-cli.md for new flags.
Done when: `node --check cli/colmeia.mjs` passes.

## §2 Operaria-Daemon — files: daemon/** only
- 1: path_windows.go: add %APPDATA%\npm and ~/.opencode/bin to card PATH if missing.
- 4/13: wait per §0 (multi ids, mode, status/exitCode/lastLine).
- 2: built-in presets claude-haiku, claude-sonnet, claude-opus (worker, with worker rules like other worker presets) in store.go.
- 11: spawn.go model for claude presets (`--model`); opencode already.
- 6: on new workspace (LoadWorkspace when no notes) create notes "Contexto do Workspace", "Comandos", "Decisoes", "Handoff" with short English templates (key: value skeleton).
- 20: `send` right after spawn: terminal DA reply `[?1;2c` lands in shell input. Find root cause; fix (e.g. send waits until card output quiet after spawn; or drop DA/CPR replies that arrive before shell ready). Don't break TUIs (no Esc to claude cards).
Done when: `cd daemon; go build ./... ; go vet ./...` pass.

## §3 Operaria-Docs — files: docs/ORCHESTRATOR.md, docs/WORKER.md, docs/agents/notas-por-workspace.md, docs/agents/playbooks/sistema-windows.md (new)
- 16: manager→user in the user's language (last message), not fixed pt-br; agent↔agent + agent notes English only (reinforce in WORKER.md + notas doc).
- 17: in Colmeia, ask as numbered text questions (answer "1: a, 2: b"); don't use AskUserQuestion dialog.
- 18: after finishing a request, append new app limitations to note "Melhorias Colmeia" (only if any).
- 7: playbook sistema-windows.md: read-only diagnosis → user approval → fix → verify; admin limits; no project dir → use docs/dev of workspace.
- Mention new CLI: wait multi/status, spawn --model, claude-haiku/sonnet/opus presets, stdin BOM-safe.
- Keep ORCHESTRATOR.md concise (it is loaded every manager turn).

## §4 Verificador → §5 Revisor (Opus, git diff) → §6 build: version 0.1.3, `npm run dist:win` → dist\Colmeia-Setup-0.1.3.exe

## §7 Item 22 (P0, user request) — Operaria-Daemon — files: docs/ORCHESTRATOR.md, daemon/internal/store/store.go (claude-manager preset), docs/WORKER.md if needed
- ORCHESTRATOR.md, short hard rule near top: all delegated work runs in Colmeia cards (spawn/reuse): agents, scouts, builds, tests, npm/go/installs. Never use built-in subagents (Agent/Task tool). Own shell only for `colmeia ...`, notes, plan.md. Prefer the user's presets from the Agentes tab (`colmeia presets`); create cards/presets when missing.
- claude-manager preset: launch claude with `--disallowedTools Agent` (check exact flag syntax with `claude --help`; keep existing args). Also migrate existing saved claude-manager preset if presets are persisted (only add flag if absent).
Done when: go build/vet/test ./... pass.

## §8 Item 23 (P0, user request) — files: cli/colmeia.mjs, daemon/** (presets API/store), docs/ORCHESTRATOR.md, docs/agents/referencia-cli.md
- Rule in ORCHESTRATOR.md (merge with §7 rule): workers ONLY from named agent presets (Agentes tab / `colmeia presets`). Never spawn a generic `shell` card and type claude/opencode in it. Missing model → create a named preset first.
- New CLI: `colmeia preset new <title> --tool claude|opencode --model <m> [--worker]` → persisted like presets created in the Agentes menu (same store, same fields), appears in the UI tab. `colmeia preset rm <id>`.
- `colmeia spawn <preset-title-or-id>` accepts the preset title (e.g. `Apis-A`).
- Build/test commands go to a named agent card (e.g. Haiku preset) acting as Verificador.
Done when: go build/vet/test ./... pass, node --check cli/colmeia.mjs passes.

## §9 Item 29 (P0, ship in 0.1.3) — file: docs/ORCHESTRATOR.md only
- Add to the existing hard rule block (§7/§8 rule): manager never starts background shell tasks of its own CLI (e.g. Claude Code run_in_background), not even to `colmeia wait`; waits with foreground `colmeia wait <id...> --timeout 590` and repeats if needed. Everything the user sees running must be a Colmeia card.
- Update the Fluxo step that says to dispatch then `wait` accordingly. Keep it short (1-2 lines).

## §10 Pre-release fixes (items 26-31) — Operaria-Presets
- 26: `colmeia --version` / `-v` / `version` → prints app version (from package.json of the install or daemon ping).
- 27: `colmeia agents set <model> auto` removes the override (back to built-in cost / `?`). Document.
- 28: shell card echo shows ✓ as V: PowerShell card input/echo not UTF-8. Make shell cards start in UTF-8 (e.g. daemon sets console code page 65001 / PowerShell [Console]::InputEncoding/OutputEncoding before prompt) without visible noise; must not affect claude/opencode cards.
- 30: workers share Claude auto-memory with manager (same cwd). In docs/ORCHESTRATOR.md: memory rules the manager saves about itself must start with "Manager only:". In docs/WORKER.md: "rules in memory/notes about the manager (Gerente) don't apply to you; run the commands your task asks".
- 31: `result` must drop trailing shell prompt lines (PS ...>, bash/zsh prompts) after the RESUMO block (cli and daemon LastResumo if shared logic). Add test.
Done when: node --check cli/colmeia.mjs, npx tsc --noEmit, daemon go build/vet/test ./... pass.

## §11 Batch 2 (user 2026-11-17), before releasing 0.1.3
### §11A Default agents (Operaria-Presets: daemon/**, cli/**)
- Built-in agent presets: ONLY "Gerente" (claude-manager) and "Revisor" (claude opus, reviewer role). Remove built-in claude-haiku/sonnet/opus (keep `shell` terminal, it's not an agent). `spawn --model` stays.
- Both built-ins are editable like user presets (label, tool, model, color, args); user edits persist (override file/fields) and survive app updates; manager rules (WORKER/ORCHESTRATOR injection, --disallowedTools Agent) are kept whatever the user changes. Option to reset to default.
- Migration: if user already has a user preset labeled "Revisor", don't create a duplicate label (reuse it as the built-in reviewer or name built-in "Revisor (padrão)" — choose the least surprising, document).
### §11C Auto layout (Operaria-Presets: daemon/**, cli/**)
- Cards spawned via CLI and notes created via CLI/seed get positions in a tidy grid (cards in a row/grid near the manager, notes in their own column/area; no overlap; consistent spacing; sizes uniform).
- `colmeia tidy` re-arranges all cards + notes of the current workspace the same way (manager first, then workers grouped by role/link, then notes). UI must update live (emit layout event).
### §11D UI (Operaria-UI: src/** only, electron/** if needed)
- Agentes tab: Gerente and Revisor shown and editable like other agents (tool, model, color, label), with "restaurar padrão".
- Right-click on a card → context menu "Criar link". Enters link mode: each next click on another card toggles/creates a link with the source card (visual hint: source highlighted, cursor/banner "Clique nos cards para ligar · Esc para sair"). Esc or click on empty canvas exits. Use the existing link create API.
- Handle live layout event from §11C.
### §11B Token economy rules (Verificador/Haiku: docs/ORCHESTRATOR.md, docs/WORKER.md only)
- Manager: delegate every action to workers (smaller models); manager only plans, decides and gives final answer. To the user: direct, short, no filler; long only if user asks for detail. Keep ORCHESTRATOR concise.
- Workers: speak only when necessary; default reply "ok"/"done"/"yes"/"no"; RESUMO only when the manager asks for one, ≤3 lines; never explain. Agent↔agent and agent notes: English.
- Manager keeps canvas organized: use `colmeia tidy` after creating cards/notes.
Done when: node --check, npx tsc --noEmit, daemon go build/vet/test ./... pass.

## §12 (user OK 2026-11-17) before 0.1.3 release
### §12.1 Windows release workflow — Operaria-UI — file: .github/workflows/release-windows.yml (new) only
- Mirror release-linux.yml style/trigger (tag push v*; same release). runs-on windows-latest; setup-node 20 + setup-go per go.mod; npm ci; npm run dist:win; upload dist/Colmeia-Setup-*.exe and dist/Colmeia-*-portable.exe to the release of that tag (create if missing, don't clobber linux assets; both workflows must coexist without race: use gh release upload --clobber or action with append).
- Pin action versions. No secrets beyond GITHUB_TOKEN. Validate YAML (node -e with js-yaml from node_modules if present).
### §12.2 Item 25 persist card --model — Operaria-Presets — daemon/** (src/** only if card json type lives there; keep minimal)
- Store model in the workspace card record when spawned with --model; restart/reopen/daemon restart use it. Old workspaces without the field keep working. Add test.
Done when: daemon go build/vet/test, npx tsc --noEmit pass.
