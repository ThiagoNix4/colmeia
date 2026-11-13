# Arquitetura do Colmeia

Canvas espacial onde cada card é um terminal real (PTY). Um card **Gerente** (Claude com `docs/ORCHESTRATOR.md`) cria e comanda os demais pela CLI `colmeia`.

## Stack
Electron + electron-vite, React 19, `@xyflow/react` (canvas), `@xterm/xterm` (UI), zustand, TypeScript estrito. Backend: daemon Go `colmeiad` (`daemon/`, PTY via `creack/pty`).

## Processos
Fluxo: Electron UI (`src/`) → preload IPC → `electron/main.ts` (ponte fina, `daemonClient.ts`) → daemon Go `colmeiad` (`daemon/`) ← CLI `cli/colmeia.mjs`.
- **colmeiad** (Go): dono dos PTYs, workspaces, notas e do socket Unix `$XDG_RUNTIME_DIR/colmeia.sock`. Os processos dos cards sobrevivem ao app.
- **Main** (Node): só ponte para o daemon (`main.ts`, `daemonClient.ts`), janela e IPC.
- **Preload**: `window.canvas` (`pty`, `app`, `presets`, `ws`) via `contextBridge`.
- **Renderer**: React Flow + xterm por card; autosave do workspace com debounce de 1 s.
- **CLI** (`cli/colmeia.mjs`): processo separado; uma linha JSON por conexão no socket.

## Arquivos-chave
| Arquivo | Papel |
|---|---|
| `electron/types.ts` | Contrato: `Preset`, `CardMeta`, `CardInfo`, `Workspace`, `CanvasEvent` |
| `electron/main.ts`, `preload.ts`, `daemonClient.ts` | Janela, IPC e cliente do socket do daemon |
| `daemon/` | colmeiad: PTY (spawn, replay, resize, send, read, wait, kill), socket, presets/workspaces/notas em `~/.config/colmeia` |
| `src/App.tsx`, `src/store.ts` | Canvas, eventos da CLI, estado zustand |
| `src/nodes/*.tsx`, `src/components/*.tsx`, `src/styles.css` | Cards, barras, estilo |

## Dados
`~/.config/colmeia/`: `presets.json`, `workspaces/<nome>.json`, `state.json`, `notes/<ws>.json`, `history/<id>.log` (≤ 512 KB). `XDG_CONFIG_HOME tem prioridade. Socket: `$XDG_RUNTIME_DIR/colmeia.sock` (0600).

## Socket
JSON por linha: `{id, cmd, args}` → `{id, ok, result|error}`. Comandos: `ping presets list sessions spawn send key broadcast read wait link focus screenshot kill detach` e `note.*`.

## Rodar
`npm install` · `npm run dev` · `npm run build` · `npm run install-app` · `npm run dist` (AppImage + deb). Requer Node 20+ e Go.

## Armadilhas
- Single instance; segunda instância sai.
- O autosave do renderer pode sobrescrever escrita externa → metadado novo vive no main, com índice próprio.
- Eventos da CLI antes do workspace carregar vão para `pendingEvents`.
- `fontReady` deve resolver antes do `pty.ensure`; xterm usa DOM (WebGL opt-in: `localStorage colmeia.webgl=1`).
- `loadWorkspace()` grava `state.json`; para varrer workspaces use `allCardIds()`.
- Erros de IPC vêm embrulhados; `ipcError()` em `App.tsx` limpa.
