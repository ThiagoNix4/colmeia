# PLAN — Persistência: terminais e workspaces sobrevivem a fechar/atualizar

## Objetivo
Fechar o Colmeia (ou reconstruir/atualizar) **não pode** perder trabalho:
1. **Sessões vivas**: cada card roda dentro de uma sessão **tmux destacada**. Fechar o app
   mata só o cliente tmux; o `claude`/`opencode` continua rodando. Ao reabrir, o card
   reconecta na mesma sessão, exatamente onde parou.
2. **Histórico em disco**: o scrollback de cada card é gravado em disco e reproduzido ao
   reabrir. É a rede de segurança para card cujo processo realmente saiu (ou quando o
   tmux não está instalado).
3. **Workspace sem perda de última hora**: o autosave tem debounce de 1s; hoje uma
   mudança nesse último segundo antes de fechar é perdida. Precisa de flush no quit.

## Estado atual (o que existe hoje, já verificado)
- `electron/store.ts` persiste o workspace em `~/.config/colmeia/workspaces/<nome>.json`
  (cards com id/preset/title/cwd/x/y/width/height, edges, viewport). **Isso já funciona.**
- `electron/ptyManager.ts` guarda o scrollback em `Session.replay` (array, cap 512KB)
  **só em memória** — nunca vai para o disco.
- `electron/main.ts` chama `ptys.killAll()` em `window-all-closed` **e** em `before-quit`
  → o shell e o que roda dentro dele morrem.
- `src/App.tsx` faz autosave do workspace com debounce de 1s (`saveTimer`).

## Restrição de ambiente
`tmux` **não está instalado** (disponível: extra/tmux 3.8; instalar exige sudo, o usuário
faz isso). Portanto: **detecção em runtime com fallback**. Sem tmux, o app funciona
exatamente como hoje, só com o histórico em disco. Nada pode quebrar por falta do tmux.

---

## 1. electron/tmux.ts (ARQUIVO NOVO)
Camada fina sobre o binário do tmux. Servidor **próprio** (`-L colmeia`), isolado do tmux
pessoal do usuário. Exporta exatamente:
```ts
export function available(): boolean              // cacheia o resultado; false se o binario nao existe
export function sessionName(cardId: string): string   // `colmeia-${cardId}`
export function confPath(): string                // escreve/garante o tmux.conf e devolve o caminho
export function listSessions(): string[]          // ids de card com sessao viva (sem o prefixo)
export function hasSession(cardId: string): boolean
export function killSession(cardId: string): void // nao lanca se nao existir
export function newSessionArgs(o: {              // argv para pty.spawn('tmux', args)
  cardId: string; cwd: string; cols: number; rows: number; shell: string; shellArgs: string[]
}): string[]
```
- `available()`: procura o binário no PATH (`which`/`execFileSync('tmux', ['-V'])` com
  try/catch). Nunca lança.
- `listSessions()` / `hasSession()` / `killSession()`: `execFileSync` com o socket
  `-L colmeia`, stdio capturado, try/catch devolvendo vazio/no-op. Nunca lança.
- `newSessionArgs()` devolve:
  `['-L','colmeia','-f',confPath(),'new-session','-A','-s',sessionName(id),'-x',String(cols),'-y',String(rows),'-c',cwd,'--',shell,...shellArgs]`
  `-A` é o coração: **anexa se existir, cria se não existir**.
- `confPath()` escreve `~/.config/colmeia/tmux.conf` (escrita atômica .tmp+rename,
  reescrever sempre para pegar atualização) com exatamente:
```
set -g status off
set -g history-limit 50000
set -g escape-time 0
set -g exit-empty off
set -g destroy-unattached off
set -g default-terminal "xterm-256color"
set -as terminal-features ",*:RGB"
setw -g aggressive-resize on
```
`status off` é obrigatório: senão a barra do tmux come uma linha e desalinha o card.

## 2. electron/historyStore.ts (ARQUIVO NOVO)
Scrollback em disco, um arquivo por card: `~/.config/colmeia/history/<cardId>.log`.
```ts
export const HISTORY_LIMIT = 512 * 1024
export function historyPath(cardId: string): string
export function readHistory(cardId: string): string       // '' se nao existir; nunca lanca
export function writeHistory(cardId: string, text: string): void  // atomico, corta no limite
export function removeHistory(cardId: string): void
export function pruneHistory(keepIds: string[]): void     // apaga .log de card que nao existe mais
```
- `writeHistory` grava o buffer **inteiro** (não é append): `.tmp` + `rename`, igual ao
  `writeJson` de `electron/store.ts`. Se passar de `HISTORY_LIMIT`, mantém só o **fim**
  do texto (`text.slice(-HISTORY_LIMIT)`).
- Toda leitura/escrita em try/catch. Falha de disco nunca derruba o app.

## 3. electron/ptyManager.ts
Mudanças:
- `spawn()`: montar o `script` do preset como hoje (`{docs}`, `keepShell`, `exec $SHELL -l`).
  Depois, **se `tmux.available()`**, trocar o alvo do `pty.spawn`:
  - hoje:  `pty.spawn(shell, args, {...})`
  - tmux:  `pty.spawn('tmux', tmux.newSessionArgs({ cardId, cwd, cols, rows, shell, shellArgs: args }), {...})`
  Mesmo `env` e mesmo `cwd` dos dois jeitos. Guardar `s.persistent = tmux.available()`.
- **Histórico**: `Session` ganha `historyTimer`. No `proc.onData`, depois de atualizar
  `replay`, agendar flush com **debounce de 2s** chamando
  `history.writeHistory(id, s.replay.join(''))`. Não gravar a cada chunk.
- `ensure()`: quando **não** há sessão em memória (app reaberto), antes de spawnar leia
  `history.readHistory(req.id)`. Se vier texto, devolver como `replay`, acrescentando no
  fim o separador em cinza:
  `'\r\n\x1b[2m── histórico anterior · reconectando ──\x1b[0m\r\n'`
  (se o histórico for vazio, `replay: ''` como hoje). O `replay` do tmux vem depois,
  naturalmente, quando o tmux redesenha a tela.
- **`detach(id)` (NOVO)**: mata só o PTY local (o cliente tmux) e remove da memória,
  **sem** `tmux kill-session`. Antes de sair, faz flush **síncrono** do histórico.
  Sem tmux, `detach` é igual a `kill`.
- **`detachAll()` (NOVO)**: `detach` em todos.
- `kill(id)`: continua matando de vez — e agora também `tmux.killSession(id)` e
  `history.removeHistory(id)`. É o caminho do usuário fechando o card de propósito.
- `killAll()`: mantém o comportamento de matar de vez (usado só onde a intenção é destruir).
- `restart(id)`: tem de matar a sessão tmux antes de recriar, senão o `-A` reanexa a
  sessão velha e o "reiniciar" não reinicia nada. Use `kill(id)` (que já mata a sessão)
  e depois `spawn`.
- **`adopt()` / reconciliação (NOVO)**: exportar
  `pruneSessions(keepIds: string[]): void` que mata sessões tmux cujo id de card não está
  em `keepIds` e chama `history.pruneHistory(keepIds)`. Evita acumular sessão órfã para
  sempre quando o usuário apaga um workspace.
- `info()`/`CardInfo`: incluir `persistent: boolean` (tem sessão tmux por trás).

Atenção ao que **não** pode regredir: `send()` com bracketed paste, `resize()`,
`read()` (texto limpo do headless) e `waitIdle()` continuam funcionando igual — o tmux
fica transparente para eles. Se algo do bracketed paste mudar dentro do tmux, mantenha o
comportamento atual e registre no RESUMO.

## 4. electron/main.ts + preload + apiServer + types
- `electron/types.ts`: `CardInfo` ganha `persistent: boolean`. `CanvasEvent` não muda.
- **O quit não pode mais matar os terminais**:
  - `app.on('window-all-closed')` → `ptys.detachAll()` (era `killAll`)
  - `app.on('before-quit')` → `ptys.detachAll()` (era `killAll`)
  - `ipcMain.on('pty:killAll')` → continua `killAll` (é ação explícita do usuário)
- **Flush do workspace no quit** (resolve a perda do último segundo). Handshake:
```ts
let flushed = false
app.on('before-quit', (e) => {
  if (!flushed && win && !win.isDestroyed()) {
    e.preventDefault()
    send('app:flush')                         // renderer salva e responde
    setTimeout(() => { flushed = true; app.quit() }, 600)   // trava de segurança
    return
  }
  ptys.detachAll()
})
ipcMain.on('app:flushed', () => { flushed = true; app.quit() })
```
  A trava de 600ms é obrigatória: se o renderer não responder, o app **tem** que fechar.
- **Na inicialização**, depois de `app.whenReady()`: juntar os ids de card de **todos** os
  workspaces (`store.listWorkspaces()` + `store.loadWorkspace(n).cards`) e chamar
  `ptys.pruneSessions(ids)`. Cuidado: `loadWorkspace` grava `state.json -> last`; para não
  trocar o workspace ativo, salve o `last` antes e restaure depois, OU leia os JSONs
  direto do disco. **Prefira ler os JSONs direto** (adicione um helper em `store.ts` se
  precisar — `store.ts` é seu nesta tarefa).
- `electron/preload.ts`: expor `app.onFlush(cb)` (listener de `app:flush`) e
  `app.flushed()` (`ipcRenderer.send('app:flushed')`).
- `electron/apiServer.ts`: `ApiContext` ganha nada novo. Comandos novos:
```
sessions {}            -> [{ id, title, alive, persistent }]  (de ctx.ptys.list())
detach   { id }        -> { id, detached: true }   (ptys.detach — card sai da tela, sessao vive)
```
  `detach` emite `{ type:'removeCard', id }` para o card sair do canvas sem matar a sessão.
- `src/App.tsx`: registrar o listener de flush:
  `window.canvas.app.onFlush(() => { <salvar workspace agora, sem debounce>; window.canvas.app.flushed() })`
  Cancelar o `saveTimer` pendente antes de salvar. Também salvar em `beforeunload`.

## 5. cli/colmeia.mjs
```
colmeia sessions            lista os cards e se têm sessão persistente (tmux)
colmeia detach <id>         tira o card da tela mantendo a sessão viva
```
Acrescentar as duas linhas no `HELP`, no alinhamento existente. `sessions` imprime
id, título, estado e `tmux` ou `-`, com `padEnd` igual ao `list`.

## Pronto quando
- `npm run build` e `npx tsc --noEmit -p tsconfig.json` limpos;
- **sem tmux instalado**: o app abre e funciona como hoje, e o card reaberto mostra o
  histórico anterior;
- **com tmux instalado**: fechar o app deixa `tmux -L colmeia ls` mostrando as sessões, e
  reabrir reconecta o card na sessão viva;
- `colmeia kill <id>` mata a sessão tmux (não sobra órfã em `tmux -L colmeia ls`);
- mover um card e fechar o app imediatamente preserva a posição.

## Divisão de arquivos (NÃO invada arquivo de outro worker)
| Worker | Arquivos |
|---|---|
| Tmux-PTY | `electron/tmux.ts` (novo), `electron/ptyManager.ts` |
| Historico | `electron/historyStore.ts` (novo) |
| Ciclo-de-vida | `electron/main.ts`, `electron/store.ts`, `electron/types.ts`, `electron/preload.ts`, `electron/apiServer.ts`, `src/App.tsx` |
| CLI-Docs | `cli/colmeia.mjs` + o vault do Obsidian |
