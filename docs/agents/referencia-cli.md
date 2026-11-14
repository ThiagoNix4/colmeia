# Referência do CLI `colmeia`

Requer o app aberto (fala por socket Unix: `$XDG_RUNTIME_DIR/colmeia.sock`; override `COLMEIA_SOCK` ou `COLMEIA_SOCK_PATH`). `npm run install-app` cria o link em `~/.local/bin/colmeia`. Use `colmeia --help` como fonte final.

## Cards
| Comando | O que faz |
|---|---|
| `list` | Cards (id, título, preset, status, cwd) |
| `presets` | Presets disponíveis |
| `spawn <preset> [--title T] [--cwd D] [--near ID]` | Cria card, liga ao `--near` (padrão: você), imprime o id |
| `send <id> <texto...\|-> [--no-enter]` | Escreve no card; Enter após 250 ms |
| `key <id> <tecla>` | `ctrl-c`, `ctrl-d`, `esc`, `enter`, `tab`, `up`, `down`, `left`, `right` |
| `broadcast <texto...> [--ids a,b]` | Vários cards; sem `--ids`, todos menos você |
| `read <id> [--lines N] [--raw]` | Tela compacta, últimas N linhas (padrão 40) |
| `result <id> [--lines N]` | Do último `RESUMO:` em diante (senão, últimas 15 linhas) |
| `wait <id> [--idle 5] [--timeout 600]` | Espera ociosidade; sai 0 se ocioso, 2 se timeout |
| `ask <id> <texto...> [--idle 8] [--lines N] [--result] [--raw]` | `send` + espera + leitura; timeout 900 s |
| `link <a> <b>` / `focus <id>` / `kill <id>` | Ligar, centralizar, fechar |
| `detach <id>` / `sessions` | Soltar card mantendo o processo no daemon / listar sessões |
| `screenshot [arq.png]` / `whoami` / `ping` | Utilitários |

`<id>` aceita id exato, prefixo ou título (sem diferenciar maiúsculas). `-` como texto lê do stdin.

## Notas
`note ls | new <título> [--near ID] | read <ref> | path <ref> | write <ref> <texto|-> | append <ref> <texto|-> | rm <ref> [--file]`. Ver [notas-por-workspace.md](notas-por-workspace.md).

## Presets
`{docs}` num comando é trocado pela pasta `docs/` do app. Presets em `~/.config/colmeia/presets.json` (lidos ao abrir o app); só `shell` e `claude-manager` são embutidos, o resto vem do menu **Agentes**.

## Agentes e policy
- `agents`: inventário de TODOS os agentes do PC (CLIs no PATH, presets, `opencode models`, `ollama list`). 1 linha por agente/modelo: `tool  model  custo(free|paid|local|?)  preset-id|-`; termina com `policy: <x>`.
- `policy`: mostra a policy do workspace do card chamador; `policy free|ask|all` define (`free` só grátis; `ask` pergunta antes do 1º pago; `all` livre).

## ask x send+wait
- Pontual: `ask --result` (uma chamada).
- Longo ou com permissões: `send` + `wait`, depois `result`; responda menus com `key`/`send`.
- Mesma coisa para vários: `broadcast`.

## Erros comuns
- `card não encontrado` → `colmeia list` (título pode ter mudado).
- `preset desconhecido` → `colmeia presets`.
- `card encerrado` → processo morreu; crie outro.
- `comando desconhecido` → app anterior ao build; reinicie o app.

> **Limite do `read`/`result`:** TUIs de tela cheia (Claude Code, opencode) usam a tela
> alternativa, então o daemon não guarda scrollback delas — só a tela visível do card.
> Por isso o `RESUMO:` do worker precisa ser curto (≤ 8 linhas) e vir no fim.

## Links (permissão)
Com `COLMEIA_CARD_ID` definido, a chamada vale como o card: `send|key|read|wait|kill|detach|focus` e `note read|write|append|path|rm` exigem link. `note ls` lista só as notas ligadas (`--all` mostra todas, com `linked`). `unlink <a> <b>` solta o par. Sem link: `sem link: "<título>" não está ligado a você`.
