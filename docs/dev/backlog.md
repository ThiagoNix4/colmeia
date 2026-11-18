source: user report 2026-11-17 (managers' session). status per item: todo
1 [P0] opencode not found in shell card (CommandNotFoundException). agents: show exe path, mark ✗ if not in card PATH; card inherits user PATH (%APPDATA%\npm, ~/.opencode/bin)
2 [P1] presets: auto per model (opencode-<model>, claude-haiku/sonnet/opus) + `spawn --model <provider/model>`
3 [P0] `send` long text fails on Windows (quotes, backslashes) + DEP0190 warning → spawn without shell:true / escape args; hide warning. workaround: stdin `-`
4 [P1] `wait` can't detect failure (idle after 0s) → return ok/error + last line; or `colmeia run <id> <cmd>` w/ exit code
5 [P2] cost `?` for opencode-go/*, opencode/big-pickle → fill cost or `agents set <model> free|paid`
6 [P1] new workspace: create notes w/ templates (Contexto do Workspace, Comandos, Decisoes, Handoff)
7 [P2] non-code tasks: playbook docs/agents/playbooks/sistema-windows.md (read-only diag → approval → fix)
10 [P1] `result` includes TUI junk (opencode footer, prompt line) → cut at end of RESUMO block
11 [P1] opencode card model choice: `spawn <preset> --model` via TUI /models or config (same as 2)
12 [P2] admin card: `spawn --admin` (UAC)
13 [P1] `wait <id1> <id2> [--all|--any]`
14 [P1] `read` strip ANSI escapes (`--raw` keeps)
15 [P2] `note append <ref> -` stdin: confirm/document
16 [P1] language: agent↔agent + agent notes English; manager→user in user's language (replace fixed "pt-br" in manager prompt); reinforce in docs/WORKER.md + note templates
17 [P1] AskUserQuestion dialog breaks in card → manager prompt: numbered text questions
18 [P2] manager prompt rule: after each request, append new app limitations to this note (if any)
19 [P1] (found by manager) PowerShell 5.1 pipe → `note append <ref> -` stdin breaks UTF-8 (BOM + mojibake on ✗/accents) → CLI decode stdin as UTF-8, strip BOM
