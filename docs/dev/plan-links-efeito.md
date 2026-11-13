# PLAN — Links com efeito (permissão + economia de token)

## Regra
Link no canvas = permissão. Vale só quando a chamada vem de um card (`from` = `COLMEIA_CARD_ID`).
Chamada sem `from` (terminal fora do Colmeia) = sem restrição. O usuário na UI também não tem restrição.

- Card A pode `send|key|read|wait|kill|detach|focus` no card B só se A e B estão ligados (link não-direcionado).
  Card sem link nenhum = "pensa sozinho": ninguém comanda.
- Card A pode `note read|write|append|path|rm` na nota N só se A e N estão ligados.
- `note ls` (com `from`) devolve **só as notas ligadas** a quem chamou → agente lê só o que precisa (economia).
  `--all` devolve todas, cada uma com `linked: boolean`.
- `list` (com `from`) devolve todos os cards com `linked: boolean` (gerente precisa descobrir cards ociosos).
- `broadcast` (com `from`, sem `ids`): só cards ligados a `from`. Com `ids`: cada id precisa estar ligado.
- Sempre liberado: `ping, presets, list, sessions, spawn, link, unlink, note.new, screenshot`.
- `spawn` já liga `near` (CLI manda `near = self`). `note.new` com `from` e sem `near` → `near = from` (nota nasce ligada a quem criou).
- Erro padrão (texto exato): `sem link: "<título>" não está ligado a você. Ligue no canvas ou rode: colmeia link <seu-id> <id>`

## Contrato técnico
- `electron/types.ts`: `CanvasEvent` ganha `| { type: 'unlink'; source: string; target: string }`.
  `CardInfo` ganha `linked?: boolean`. `NoteCard` resultado de lista pode ter `linked?: boolean` (campo opcional, não persiste).
- IPC renderer → main: `ipcRenderer.send('links:set', pairs)` com `pairs: [string, string][]` (source,target de TODOS os edges do workspace).
  preload expõe `window.canvas.links.set(pairs: [string, string][]): void`.
  Renderer chama ao carregar o workspace e a cada mudança de `edges` (inclusive troca de workspace → lista nova).
- `electron/links.ts` (novo): `setPairs(pairs)`, `linked(a, b): boolean` (não-direcionado), `neighbors(id): Set<string>`. Em memória.
- API: todo request pode ter `args.from?: string`. Novo comando `unlink` `{a, b}` → emite `{type:'unlink', source, target}` e remove o par de `links.ts` na hora (não esperar o renderer). `link` também adiciona o par em `links.ts` na hora.
- CLI: `call(cmd, args)` injeta `from: process.env.COLMEIA_CARD_ID` quando existir. Novo `colmeia unlink <a> <b>`. `note ls --all`. `list` mostra marca `[ligado]` nos cards ligados. HELP atualizado.

## Workers e arquivos (um arquivo, um dono)
| Worker | Modelo | Arquivos |
|---|---|---|
| Main | sonnet | `electron/links.ts` (novo), `electron/apiServer.ts`, `electron/main.ts`, `electron/preload.ts`, `electron/types.ts` |
| UI+CLI | sonnet | `src/App.tsx`, `src/store.ts`, `cli/colmeia.mjs` |
| Docs | haiku | `docs/WORKER.md`, `docs/ORCHESTRATOR.md`, `docs/agents/referencia-cli.md`, `docs/agents/economia-de-token.md` |

Docs: explicar a regra acima em poucas linhas. WORKER.md: "Ao começar, rode `colmeia note ls`: lê só as notas listadas (as ligadas a você). Nenhuma listada = não leia nota." ORCHESTRATOR.md: gerente liga ao worker só as notas de que ele precisa (`colmeia link <worker> <nota>`), e para comandar card que não criou precisa ligar antes (`colmeia link <eu> <card>`); `colmeia unlink` para soltar.

## Pronto quando
`npx tsc --noEmit -p tsconfig.json` limpo e `node --check cli/colmeia.mjs` ok. Gerente valida no app.
