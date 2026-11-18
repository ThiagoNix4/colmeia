# Review — plan-melhorias §10 (items 26-31)

Scope: git diff for spawn.go (UTF-8 shell start), `agents set auto`, `--version`, `result` prompt trim (CLI + daemon `links.LastResumo`), docs.
Verdict: 3 bugs to fix before release; rest OK.

## Bugs

### 1. `cli/colmeia.mjs:138` — `RESUMO_END` `/^\s*build\b/i` truncates real RESUMO lines
- Meant to stop at the opencode footer (`Build · <model>`), but matches any line starting with the word "build".
- Verified: `"  build/vet/test ok"` → matches → block cut. Workers often report "build/vet/test pass" right after `RESUMO:`.
- Fix: `/^\s*build\s+·/i` (footer shape only).

### 2. `daemon/internal/links/links.go:174` — `promptLine` ends the RESUMO block on normal content
- `^(PS [^>]*>|\S*[$%#>])\s*$` matches any single token ending in `$ % # >`; the second alternative `^PS [^>]*>` is unanchored at the end.
- Verified matches (all real-content lines that would cut the block / `--pipe` forward): `95%`, `C#`, `->`, `PS fix a->b`.
- Same problem in CLI: `cli/colmeia.mjs:137` `SHELL_PROMPT` (less harm, only trailing pop) and `cli/colmeia.mjs:138` `RESUMO_END` entry `/^\s*PS [^>]*>/`.
- Fix: anchor to real prompt shapes, shared in both places, e.g.
  `^PS [A-Za-z]:\\[^>]*>\s*$|^\S+@\S+:\S*[$#%]\s*$|^[$%#>❯]\s*$`
  Add tests: `RESUMO: x\n95%` and `RESUMO: x\nPS fix a->b` keep the second line.

### 3. `daemon/internal/api/spawn.go:171` — UTF-8 init forced on non-PowerShell shells
- The plain-shell branch always uses `-NoLogo -NoExit -Command "$null=chcp 65001; [Console]::..."` on Windows.
- `term.DefaultShell()` honors `COLMEIA_SHELL`; with cmd.exe or bash.exe the card fails to start or prints errors (visible noise). Before this change the plain card used `shellArgs` only.
- Fix: apply only when `strings.ToLower(filepath.Base(shell))` is `pwsh.exe`/`powershell.exe` (or starts with `pwsh`/`powershell`); else keep `shellArgs`.

## Verified OK
- UTF-8 init command (item 28): ran in PS 5.1 — chained assignment works; `[Console]::OutputEncoding`, `$OutputEncoding`, `[Console]::InputEncoding` all `utf-8`, preamble length 0 (no BOM), no output. Preset (claude/opencode) cards unaffected (separate branch).
- `agents set <model> free|paid|auto` (item 27): `auto` deletes the override; `costOf` falls back to built-in rules. Documented in `docs/agents/referencia-cli.md:34`.
- `--version`/`-v`/`version` (item 26): reads package.json near the CLI, falls back to daemon `ping` (`version` field exists in `cmds.go:290`). Documented in `referencia-cli.md:35`.
- Item 30 docs: `docs/ORCHESTRATOR.md:127` ("Manager only:" rule), `docs/WORKER.md:44` (manager memory rules don't apply to workers).
- Item 31 tests exist in `links_test.go` (PS and bash prompt), but they need the negative cases from bug 2.
