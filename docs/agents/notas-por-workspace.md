# Notas por workspace

Contexto por trabalho, lido sem gastar token.

- Nota = arquivo `.md` real, ligado a um workspace e opcionalmente a cards no canvas. Mesma fonte para o canvas e para editores externos (ex.: Obsidian). Não é PTY.
- Arquivo: `<notesRoot>/<workspace>/<slug>.md`. Índice: `~/.config/colmeia/notes/<workspace>.json`.
- `notesRoot`, em ordem: `$COLMEIA_NOTES_DIR` → `root` em `~/.config/colmeia/notes-config.json` → vault Obsidian aberto + `/Colmeia` → `~/.config/colmeia/notes-files`.
- `slug`: título em minúsculas, `[^\w.-]+` → `-`, máx. 64; repetido ganha `-2`, `-3`.

## Uso
```sh
colmeia note ls                  # notas do workspace
colmeia note read <ref>          # texto cru
colmeia note append <ref> "..."  # ou "-" para stdin
colmeia note write <ref> "..."   # sobrescreve
```
`<ref>` = id, prefixo ou título.

## Convenções
- Uma nota por tarefa/frente, título curto. Comece lendo a nota do workspace; termine atualizando (resultado, arquivos, próximos passos).
- Conhecimento reutilizável vai para `docs/agents/` (versionado); o específico do trabalho fica na nota. Não duplique: referencie.
- Workspace atual: seletor no canto do app, ou `~/.config/colmeia/state.json` (`last`). O nome do arquivo é sanitizado (`[^\w.-]+` → `_`).

## Idioma/estilo (economia de token)
Inglês telegráfico: é o idioma mais barato no tokenizer do Claude (pt-br ~+30–50% tokens; chinês/japonês não economizam, ~1+ token por caractere) e cortar gramática rende mais que trocar de idioma.
- Sem artigos, cortesia, frases completas. Fragmentos, `chave: valor`, listas.
- Símbolos: `→` causa/próximo, `=` é, `✓` feito, `✗` falhou/não, `~` aprox., `+`/`-` add/remover.
- Abreviações comuns: cfg, dir, impl, dep, fn, msg, req, env, ws (workspace).
- Caminhos/comandos literais, sem explicar o óbvio. Títulos podem ficar em pt-br (sem acento).
- Ex.: `build: npm run build → out/. ✗ run while app open (locks pty).`
