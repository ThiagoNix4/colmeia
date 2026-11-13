# PLAN — Gerenciar workspaces + remover preset DeepSeek

Projeto: `colmeia/` (raiz do repo) (Electron + React + xyflow + node-pty)

## Objetivos
1. **Remover o preset `opencode-deepseek`** de todo o app (default, config já persistida, docs).
   Manter `shell`, `claude`, `claude-manager`, `opencode`.
2. **Workspaces: além de criar, poder RENOMEAR e EXCLUIR.**
   Hoje a Toolbar só tem um `<select>` + botão `＋`.

---

## Contrato de API (congelado — ambos os lados programam contra isto)

### `electron/store.ts`
```ts
export function deleteWorkspace(name: string): void
// remove <config>/colmeia/workspaces/<safeName>.json; no-op se não existir.

export function renameWorkspace(from: string, to: string): string
// renomeia o arquivo e devolve o novo nome sanitizado.
// lança Error('workspace não encontrado') se `from` não existe
// lança Error('já existe um workspace com esse nome') se o destino existe
// se `from` era o último aberto (state.json), atualiza state.last para o novo nome
```

### IPC (`electron/main.ts`)
```ts
ipcMain.handle('ws:delete', (_e, name: string) => store.deleteWorkspace(name))
ipcMain.handle('ws:rename', (_e, from: string, to: string) => store.renameWorkspace(from, to))
```

### `electron/preload.ts` — dentro de `api.ws`
```ts
remove: (name: string): Promise<void> => ipcRenderer.invoke('ws:delete', name),
rename: (from: string, to: string): Promise<string> => ipcRenderer.invoke('ws:rename', from, to)
```

### Props do `Toolbar` (`src/components/Toolbar.tsx`)
```ts
interface Props {
  onNewCard: (presetId: string) => void
  onSwitchWorkspace: (name: string) => void
  onCreateWorkspace: (name: string) => void              // cria e já troca para ela
  onRenameWorkspace: (from: string, to: string) => Promise<void>
  onDeleteWorkspace: (name: string) => Promise<void>
}
```
`App.tsx` implementa esses 3 novos handlers (feito pelo Gerente).

---

## Divisão de tarefas — NINGUÉM edita arquivo de outro

### Worker BACKEND  →  `electron/store.ts`, `electron/main.ts`, `electron/preload.ts`
- `DEFAULT_PRESETS`: apagar a linha do `opencode-deepseek`.
- Migração em `loadPresets()`: const `REMOVED_PRESET_IDS = ['opencode-deepseek']`.
  Filtrar esses ids da lista lida do disco; se algo foi removido, reescrever o `presets.json`
  (preserva as outras customizações do usuário).
- Migração em `loadWorkspace()`: card com `preset === 'opencode-deepseek'` passa a `'opencode'`.
- `deleteWorkspace` e `renameWorkspace` conforme o contrato.
- Registrar os 2 handlers IPC em `main.ts` e os 2 métodos em `preload.ts`.

### Worker FRONTEND  →  `src/components/Toolbar.tsx`, `src/styles.css`
- Trocar o bloco de workspace por um controle com 3 ações, usando as props novas:
  `＋` novo · `✎` renomear o atual · `🗑` excluir o atual.
- Renomear: vira um `<input>` inline pré-preenchido com o nome atual (Enter confirma, Esc cancela),
  igual ao fluxo de criar que já existe.
- Excluir: `window.confirm('Excluir o workspace "X"? Os cards salvos nele serão perdidos.')`.
  O botão 🗑 fica `disabled` quando `workspaces.length <= 1`.
- Sanitizar nomes com `.replace(/[^\w.-]+/g, '_')` e ignorar vazio, como já é feito hoje.
- CSS: estilo dos botões de ação (`.ws-actions button`), hover, e um vermelho discreto no 🗑.
- NÃO editar `App.tsx` nem `src/store.ts`.

### Gerente (eu)  →  `src/App.tsx`, `src/store.ts`, `README.md`, `docs/ORCHESTRATOR.md`
- Handlers `onCreateWorkspace` / `onRenameWorkspace` / `onDeleteWorkspace` em `App.tsx`,
  recarregando a lista de workspaces no store depois de cada operação.
- Excluir o workspace atual → trocar para o primeiro restante (ou criar `default` vazio).
- Docs: tirar DeepSeek, documentar renomear/excluir.
- Build (`npm run build`) e revisão final.

## Pronto quando
`npm run build` passa sem erro de TypeScript, nenhuma menção a deepseek fora de `.tasks/*.log`,
e a Toolbar permite criar, renomear e excluir workspace.
