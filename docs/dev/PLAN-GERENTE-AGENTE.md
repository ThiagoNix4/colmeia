# PLAN — Escolher a IA do Gerente no menu Agentes

Worker: **Main** (Sonnet). Arquivos permitidos: `electron/store.ts`, `electron/types.ts`, `src/components/AgentsMenu.tsx` (+ `src/styles.css` se precisar).

## Objetivo
O preset embutido `claude-manager` (Gerente) continua builtin (não exclui, não muda id/nome/cor), mas o usuário escolhe **ferramenta + modelo** no menu Agentes ▾, como nos outros agentes.

## Backend (`electron/store.ts`, `types.ts`)
- `Preset` ganha `configurable?: boolean` (true só no Gerente).
- Comando do Gerente gerado por `managerCommand(tool, model)` (`O = "$(cat '{docs}/ORCHESTRATOR.md')"`, `[x]` só se modelo não vazio):
  - claude:   `claude [--model M] --effort medium --append-system-prompt O`  (sem modelo = comando atual, idêntico)
  - opencode: `opencode [-m M] --prompt O`
  - codex:    `codex [-m M] O`
  - gemini:   `gemini [-m M] -i O`
- Persistência: entrada `{id:'claude-manager', tool, model}` em presets.json. `loadPresets` aplica tool/model salvos ao builtin (default tool=claude); `savePresets` aceita a entrada do Gerente mesmo com `builtin:true`, guarda só tool (claude|opencode|codex|gemini, sem custom) e model, e recalcula o comando. Shell continua intocável.

## Frontend (`AgentsMenu.tsx`)
- Linha do Gerente ganha botão editar (✎). Form reduzido: select de ferramenta (sem "Personalizado"), campo modelo, preview do comando só-leitura. Nome/cor/excluir não aparecem.
- Salvar envia a lista atual com a entrada do Gerente atualizada.

## Pronto quando
`npx tsc --noEmit -p tsconfig.json` limpo; presets.json antigo (sem tool no Gerente) carrega com o comando atual igual.
