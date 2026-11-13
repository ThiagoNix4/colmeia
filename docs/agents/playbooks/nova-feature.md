# Playbook — nova feature no Colmeia

Antes: leia [arquitetura](../arquitetura.md) e o `PLAN*.md`. Defina **um dono por arquivo**.

## Ordem
1. Contrato em `electron/types.ts`.
2. Lógica no main (módulo novo, pura).
3. IPC em `main.ts` + `preload.ts`.
4. Comando no daemon (`daemon/internal/api/cmds.go`) + CLI (valida sem UI).
5. Estado em `src/store.ts`.
6. Nó/UI + registro em `App.tsx`.
7. Toolbar e estilos.
8. Docs (`ORCHESTRATOR.md`, `README.md`, `docs/agents/`).
9. Validação.

## Padrões
- Escrita atômica (`.tmp` + `renameSync`); leitura de arquivo ausente devolve fallback, não lança.
- Não mude o formato de `Workspace` se um índice separado resolve (precedente: notas).
- IPC: `ipcMain.handle` p/ request/response, `ipcMain.on` p/ fire-and-forget; espelhe em `preload.ts`.
- Socket: comando no daemon Go (`daemon/internal/api/cmds.go`, dispatch em `server.go`); valide com `str()`, resolva card com `resolveCard`.
- CLI: `case` + linha no `HELP`; reuse `parse()`, `readText()`, `call()`; erro → `process.exit(1)`.
- Renderer: ações idempotentes; use `freeSpot()`; eventos pré-`ready` vão para `pendingEvents`; não crie outro save concorrente.
- Nó: siga `TerminalNode.tsx` (`memo`, `NodeResizer`, handles, `dragHandle: '.card-header'`) e as variáveis de `styles.css`.

## Divisão de workers
Por camada (backend / CLI / frontend / docs), nunca metade de um arquivo. Leitura é livre. Só o gerente integra, roda build e valida. Sem `npm install` em paralelo. Tarefa do worker: contexto, arquivos permitidos, trecho do contrato, critério de pronto, "responda `RESUMO:`".

## Validar
```sh
npx tsc --noEmit -p tsconfig.json
npm run build
node --check cli/colmeia.mjs
```
Com o app aberto: `colmeia spawn shell --title Teste`, `colmeia ask Teste "echo ok" --result`, `colmeia kill Teste`. Confira persistência ao trocar de workspace/reabrir e regressão dos comandos antigos.

## Armadilhas
- Enter do `send` tem atraso de 250 ms; para menus use `colmeia key`.
- Nome de workspace é sanitizado; não assuma o digitado.
- O daemon é Go: após mudar `daemon/`, rode `go vet ./... && go build ./...` e reinicie o `colmeiad`.
- Nota não entra em `Workspace.cards`.
- `build` não substitui `tsc`.
