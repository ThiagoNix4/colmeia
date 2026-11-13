# PLAN-AGENTES — menu de Agentes + logo nova

## Objetivo
1. Tirar os botões de preset do topo. No lugar, um botão **"Agentes ▾"** que abre um menu
   (estilo dropdown/hambúrguer, como o seletor de workspaces) com os agentes do usuário.
   No menu: clicar num agente cria o card; **"+ Novo agente"** abre um formulário; cada agente
   tem editar e excluir. Tudo salvo em `~/.config/colmeia/presets.json`.
2. O app **não vem com agentes prontos**: os únicos embutidos são `shell` (Terminal) e
   `claude-manager` (Gerente), marcados `builtin: true` (não podem ser excluídos nem editados).
   O resto o usuário cria (Claude modelo A/B/C, Codex, Gemini, opencode -m X, etc.).
3. Logo nova: minimalista, ligada a colmeia (favo hexagonal / abelha geométrica), dourada.

## Contrato de dados (`electron/types.ts`)
```ts
export type AgentTool = 'claude' | 'opencode' | 'codex' | 'gemini' | 'custom'
export interface Preset {
  id: string
  label: string
  command: string        // o que roda de fato (vazio = shell puro)
  color: string
  keepShell?: boolean
  builtin?: boolean      // shell e claude-manager; não editáveis/excluíveis
  tool?: AgentTool       // só para o formulário reabrir preenchido
  model?: string         // idem
}
```
Templates de comando (o formulário gera o `command` a partir de ferramenta + modelo; o usuário
ainda pode editar o comando final à mão):
- claude → `claude --model <modelo>` (modelo vazio → `claude`)
- opencode → `opencode -m <modelo>` (vazio → `opencode`)
- codex → `codex -m <modelo>` (vazio → `codex`)
- gemini → `gemini -m <modelo>` (vazio → `gemini`)
- custom → comando livre

## IPC (novo)
- `presets:save` (renderer → main): recebe `Preset[]` **só dos não-builtin**; o main valida
  (id único, label não vazio, cor `#rrggbb`), junta com os builtins, grava atômico (`.tmp` +
  `rename`), atualiza a variável `presets` do main (o CLI `colmeia presets`/`spawn` passa a ver
  na hora) e devolve a lista completa.
- preload: `window.canvas.presets.save(list): Promise<Preset[]>`.
- `presets:get` continua igual (lista completa, builtins primeiro).

## Divisão (arquivos disjuntos)
| Worker | Modelo | Arquivos |
|---|---|---|
| **Backend** | Claude Haiku | `electron/types.ts`, `electron/store.ts`, `electron/main.ts`, `electron/preload.ts`, `src/global.d.ts` (se tipar `window.canvas` lá) |
| **Links** (UI) | Claude Sonnet | `src/components/AgentsMenu.tsx` (novo), `src/components/Toolbar.tsx`, `src/store.ts`, `src/App.tsx`, `src/styles.css` |
| **Logo** | Claude Sonnet | `src/components/Logo.tsx` (novo), `assets/icon.svg`, `assets/icon.png` |

### Backend
- `DEFAULT_PRESETS` = só `shell` e `claude-manager`, ambos `builtin: true`.
- `loadPresets()`: garante que os dois builtins existem (sempre na versão do código, primeiro
  da lista) mesmo se o JSON do usuário não tiver; agentes do usuário vêm depois, intactos.
  Não apagar os agentes que já estão no arquivo do usuário.
- `savePresets(list)` em `store.ts` + handler `presets:save` em `main.ts` + preload.

### Links (UI)
- Toolbar: logo (`<Logo size={18} />` de `./Logo`, default export) + "Colmeia"; depois os
  botões **Terminal** (preset `shell`) e **+ Nota**; depois **Agentes ▾**. Nenhum outro botão
  de preset no topo.
- `AgentsMenu.tsx`: popover ancorado no botão, fecha com Esc/clique fora. Lista: Gerente
  primeiro (builtin), divisor, agentes do usuário (bolinha da cor + nome + comando em cinza
  pequeno; ícones editar ✎ e excluir 🗑 no hover). Estado vazio: "Nenhum agente ainda — crie
  o seu". Rodapé: "+ Novo agente".
- Formulário (dentro do popover): Nome, Ferramenta (select: Claude Code, opencode, Codex,
  Gemini CLI, Personalizado), Modelo (texto, com placeholder de exemplo por ferramenta),
  Comando (preenchido pelo template, editável; se o usuário editar à mão, para de sobrescrever),
  Cor (paleta de ~8 bolinhas + input color), checkbox "manter shell ao sair" (padrão on).
  Salvar/Cancelar. Excluir pede confirmação inline (não `window.confirm`).
- `src/store.ts`: ação para atualizar `presets` depois do save.
- Visual: mesmo estilo escuro/dourado do app, minimalista, sem poluir.

### Logo
- `assets/icon.svg`: minimalista, favo/hexágono dourado (`#e8b04a`-ish) sobre fundo escuro
  arredondado, legível em 16px e 512px. Sem texto.
- `assets/icon.png` 512×512 gerado do SVG com `rsvg-convert -w 512 -h 512`.
- `src/components/Logo.tsx`: `export default function Logo({ size = 18 }: { size?: number })`
  com o SVG inline (sem fundo, só a marca, usando `currentColor`/dourado), para a toolbar.

## Regras
- Ler `colmeia note read 'Regras dos Agentes'`. Só mexer nos seus arquivos. Programar contra
  este contrato (o arquivo do outro pode ainda não existir).
- Pronto: `npx tsc --noEmit -p tsconfig.json` limpo. Não rodar `npm run build`, não reiniciar
  o app, **não commitar**.
- Responder `RESUMO:` curto por arquivo.
