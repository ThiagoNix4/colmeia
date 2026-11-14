# Plan: escolha de agentes pelo Gerente (política de custo por workspace) → release 0.1.1

## Contrato (P0)
Workspace policy `models` ∈ `free` | `ask` | `all` (default `ask`). Stored with workspace state (same place as other per-workspace data).
- free: só agentes grátis (opencode `-free`, modelos locais, free tiers). Se pago for claramente melhor numa tarefa → Gerente PERGUNTA ao usuário.
- ask: grátis por padrão; pago quando melhor, mas pergunta antes do 1º uso pago na tarefa.
- all: acesso total; Gerente escolhe livre pelo melhor custo/benefício.

## §1 Operária-Code (Sonnet) — P1
Files: cli/colmeia.mjs, daemon/ (only what's needed for policy get/set + store), src/ workspace menu component(s) for toggle.
- `colmeia agents`: inventário de TODOS agentes no PC: CLIs no PATH (claude, opencode, codex, gemini, qwen, aider, cursor-agent, ollama, crush, goose…), presets do menu Agentes, `opencode models` (timeout 10s, cache), `ollama list`. Saída compacta 1 linha por agente/modelo: `tool  model  custo(free|paid|local|?)  preset-id|-`. Termina com `policy: <x>`.
  Custo: `-free`/`:free` sufixo, ollama/local → free/local; claude/codex/gemini/gpt/opus/sonnet → paid; resto `?`.
- `colmeia policy` mostra; `colmeia policy free|ask|all` define (workspace do card chamador).
- UI: no menu/config do workspace, seletor "Modelos: Só grátis / Perguntar antes de pago / Acesso total".
Done: tsc ✓, go test ✓, `colmeia agents` e `colmeia policy` funcionam.

## §2 Operária-Docs (Haiku) — P1
Files: docs/ORCHESTRATOR.md, docs/agents/*.md if they reference model choice, README (CLI list).
Insert section "Escolha de agentes (inventário + política)" (text in §2a) replacing/extending "Escada de modelo"; add `colmeia agents`/`colmeia policy` in CLI block.

### §2a texto
- Início de toda tarefa: `colmeia agents` (uma vez) → saber TUDO que existe no PC e a `policy` do workspace. Não se limite a Claude nem a um provedor.
- Critério = melhor resultado por custo para AQUELA tarefa:
  1. Grátis que faz igual ou melhor que um pago → grátis.
  2. Pago claramente melhor para o uso (raciocínio difícil, revisão, arquitetura, bug sem pista, código grande) → pago, se a policy permitir.
  3. Empate → mais barato/rápido; cota esgotada → equivalente de outro provedor.
- Policy: `free` → só grátis; se um pago for claramente melhor, pergunte ao usuário (1 linha: tarefa, modelo pago, por quê, alternativa grátis) e siga grátis enquanto espera. `ask` → grátis por padrão, pergunta antes do 1º uso pago na tarefa. `all` → escolhe livremente, sem perguntar.
- Usuário pode mudar: "use só grátis" → `colmeia policy free`; "pode usar tudo" → `colmeia policy all`. Registre em Decisoes.
- Escada continua valendo, mas é por capacidade, não por marca: grátis fraco → médio (grátis ou pago) → topo.

## §3 Verificador (Haiku): tsc, go vet/test, npm run build.
## §4 Revisor (Opus): git diff do §1.
## §5 Release 0.1.1 (Gerente+Verificador): bump package.json → 0.1.1, `npm run dist` (AppImage+deb) e `npm run dist:win` (wine), docs/WINDOWS.md notas, commit+push, `gh release create v0.1.1` com 4 assets.
