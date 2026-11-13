# PLAN — Cards de Nota (bloco de notas por workspace) + base no Obsidian

## Objetivo
Dar ao Colmeia um **card de nota**: um bloco de markdown que fica no canvas, pertence a
um workspace e pode ser **ligado a cards de agente**. O agente lê a nota com um comando
barato (`colmeia note read <ref>` ou `cat <path>`), em vez de receber o contexto digitado
por um humano/gerente a cada vez. Objetivo final: **gastar menos token**.

## Decisão de arquitetura (NÃO mudar)
1. **A nota é um arquivo `.md` de verdade no disco**, dentro do vault do Obsidian.
   Mesma fonte de verdade para o canvas e para o Obsidian. Sem frontmatter gerado por
   código — o arquivo é markdown puro.
2. **Nota NÃO é PTY.** Não passa pelo `PtyManager`. Tem registro próprio.
3. **O processo main é o único dono dos metadados da nota.** Renderer e CLI leem/escrevem
   pelo mesmo store. Isso evita corrida entre o `ws:save` do renderer e o CLI.
4. O formato do `Workspace` JSON existente **não muda** (sem migração, sem risco).
   Notas ficam num índice separado, por workspace.

## Caminhos
```
raiz das notas (notesRoot), resolvida nesta ordem:
  1. $COLMEIA_NOTES_DIR
  2. "root" em <config>/colmeia/notes-config.json
  3. vault do Obsidian + "/Colmeia"   <- parse de ~/.config/obsidian/obsidian.json,
                                         pega o vault com "open": true, senão o primeiro
  4. <config>/colmeia/notes-files     (fallback se não houver Obsidian)

arquivo da nota  : <notesRoot>/<workspace>/<slug>.md
índice da nota   : <config>/colmeia/notes/<workspace>.json
<config>         : $XDG_CONFIG_HOME ou ~/.config , + "/colmeia"  (igual a electron/store.ts)
```
Hoje, nesta máquina, isso resolve para `<vault>/Colmeia/<ws>/<slug>.md`.

`slug` = título em minúsculas, `[^\w.-]+` -> `-`, cortado em 64 chars, vazio -> `nota`.
Se o slug já existir, sufixa `-2`, `-3`, ...

## Tipos (electron/types.ts)
```ts
export interface NoteMeta {
  id: string            // 8 hex, mesmo gerador dos cards (newCardId)
  title: string
  file: string          // caminho ABSOLUTO do .md
  workspace: string
}
export interface NoteCard extends NoteMeta {
  x: number; y: number; width: number; height: number
}
export interface NotesIndex { workspace: string; notes: NoteCard[] }
```
E dois eventos novos no `CanvasEvent`:
```ts
  | { type: 'addNote'; note: NoteCard; near?: string }
  | { type: 'removeNote'; id: string }
```

## electron/notesStore.ts (ARQUIVO NOVO)
Exporta exatamente isto:
```ts
export function notesRoot(): string
export function noteDir(workspace: string): string          // notesRoot()/<ws>, cria se faltar
export function listNotes(workspace: string): NoteCard[]
export function getNote(workspace: string, ref: string): NoteCard | undefined
  // ref casa com: id exato -> título (case-insensitive) -> prefixo de id
export function createNote(workspace: string, title: string, body?: string): NoteCard
  // cria o .md (body ?? `# ${title}\n\n`) e grava no índice; x/y = 0, width 420, height 320
export function updateNote(workspace: string, id: string, patch: Partial<Omit<NoteCard,'id'|'workspace'>>): NoteCard
  // renomear título NÃO renomeia o arquivo (evita link quebrado no Obsidian)
export function removeNote(workspace: string, id: string, deleteFile?: boolean): void
export function readNote(workspace: string, ref: string): string      // conteúdo do .md
export function writeNote(workspace: string, ref: string, text: string): void
export function appendNote(workspace: string, ref: string, text: string): void
```
Regras: escrita atômica (`.tmp` + `rename`, igual ao `writeJson` do store.ts);
`mkdirSync(..., {recursive:true})` antes de escrever; se o `.md` foi apagado por fora,
`readNote` devolve `''` em vez de lançar erro. Nunca lançar por arquivo ausente na leitura.

## IPC (electron/main.ts) + preload
Canais novos, no mesmo estilo dos existentes:
```
notes:list   (ws)                 -> NoteCard[]
notes:create (ws, title)          -> NoteCard
notes:update (ws, id, patch)      -> NoteCard
notes:remove (ws, id, deleteFile) -> void
notes:read   (ws, ref)            -> string
notes:write  (ws, ref, text)      -> void
notes:root   ()                   -> string
```
Em `electron/preload.ts`, expor em `api.notes` com os mesmos nomes
(`list/create/update/remove/read/write/root`), todos `ipcRenderer.invoke`.

## Socket / apiServer (electron/apiServer.ts)
`ApiContext` ganha `getWorkspace: () => string` (nome do workspace ativo).
`resolveCard` continua só para PTY. Criar um `resolveAny(ctx, ref)` que tenta PTY e
depois nota — usado **somente** por `link` e `focus`, para poder ligar nota <-> agente.
Comandos novos:
```
note.list  {}                      -> NoteCard[]
note.new   { title, near?, body? } -> NoteCard   (emite addNote com near)
note.read  { ref }                 -> { ref, title, file, text }
note.write { ref, text, append? }  -> { ref, file }
note.path  { ref }                 -> { ref, file }
note.rm    { ref, file? }          -> { ref, removed:true }  (emite removeNote)
```
`note.new` também emite `{ type:'link', source: near, target: note.id }` quando `near` vier,
igual ao `spawn` faz hoje.

## CLI (cli/colmeia.mjs) — SÓ este arquivo
Subcomando `note`:
```
colmeia note ls                          lista as notas do workspace (id, título, caminho)
colmeia note new <título> [--near ID]    cria nota no canvas (imprime o caminho do .md)
colmeia note read <ref>                  imprime o conteúdo (é isso que o agente usa)
colmeia note path <ref>                  imprime só o caminho absoluto
colmeia note write <ref> <texto...|->    sobrescreve
colmeia note append <ref> <texto...|->   acrescenta no fim
colmeia note rm <ref> [--file]           remove do canvas (--file apaga o .md também)
```
`<ref>` = id, prefixo de id ou título. Reaproveitar `parse()`, `readText()` e `call()`
que já existem. Acrescentar o bloco `note` no `HELP` (mesmo estilo/alinhamento).
`note read` imprime o texto cru, sem cabeçalho — tem que dar para fazer pipe.

## Renderer (src/)
- `src/nodes/NoteNode.tsx` (NOVO): nó do React Flow, `type: 'note'`.
  - textarea de markdown, largura/altura redimensionáveis como o TerminalNode;
  - **autosave com debounce de 600ms** via `window.canvas.notes.write`;
  - cabeçalho com o título (duplo clique renomeia, igual ao TerminalNode faz) e um
    botão que copia o caminho do `.md`;
  - handles de conexão iguais aos do TerminalNode, para ligar nota -> agente;
  - visual: fundo levemente amarelado/âmbar para distinguir do terminal, fonte de leitura
    (não monoespaçada), respeitando as variáveis de cor já usadas em `src/styles.css`.
- `src/App.tsx`: registrar `note: NoteNode` em `nodeTypes`; tratar `addNote`/`removeNote`
  no `onCanvasEvent`; carregar as notas junto com o workspace.
- `src/store.ts`: `NoteNode` no union de nós; carregar/salvar posição e tamanho da nota
  por `notes:update` (NÃO enfiar nota no `Workspace.cards` — o formato não muda).
- `src/components/Toolbar.tsx`: botão "+ Nota" que chama `notes.create` e põe no canvas
  usando o mesmo `freeSpot()` dos cards.
- `src/global.d.ts`: nada a fazer (tipo vem do preload automaticamente).

## Pronto quando
- `npm run build` passa sem erro de TypeScript;
- `colmeia note new "X"` cria o `.md` no vault e o card aparece no canvas;
- `colmeia note read X` imprime o conteúdo;
- dá para ligar a nota a um card de agente arrastando no canvas e por `colmeia link`;
- editar no Obsidian e reabrir o card mostra o texto novo (e vice-versa).

## Divisão de arquivos (NÃO invada arquivo de outro worker)
| Worker | Arquivos |
|---|---|
| Backend-Notas | `electron/notesStore.ts` (novo), `electron/types.ts`, `electron/main.ts`, `electron/preload.ts`, `electron/apiServer.ts` |
| CLI-Notas | `cli/colmeia.mjs` |
| Frontend-Notas | `src/nodes/NoteNode.tsx` (novo), `src/App.tsx`, `src/store.ts`, `src/components/Toolbar.tsx`, `src/styles.css` |
| Vault | só dentro de `<vault>` |
