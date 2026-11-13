# Playbook — persistência e daemon colmeiad

Terminais e estado sobrevivem a fechar/atualizar o app porque os PTYs pertencem ao daemon Go `colmeiad`, não ao Electron.

## Estado
| Caminho | Guarda |
|---|---|
| `~/.config/colmeia/workspaces/<nome>.json` | Cards, arestas, viewport, cwd (autosave 1 s + flush no quit) |
| `~/.config/colmeia/history/<cardId>.log` | Scrollback do card (rede de segurança) |
| `~/.config/colmeia/state.json` | `{ "last": "<workspace>" }` |
| `$XDG_RUNTIME_DIR/colmeia.sock` | Socket do daemon (0600) |

## Inspecionar
```sh
colmeia sessions              # cards vivos no daemon
colmeia read <id> --lines 120 # ler sem anexar
```

## Detach x kill
| Ação | Processo | Card | Histórico |
|---|---|---|---|
| Fechar/atualizar app | vive (daemon) | volta | flush |
| `colmeia detach <id>` | vive | sai do canvas | flush |
| `colmeia kill <id>` / X | morre | removido | apagado |
| restart do card | mata e recria | fica | apagado |
| Reboot / daemon morto | morre | layout e histórico ficam | ficam |

## Flush no quit
`before-quit` → `preventDefault` + `app:flush`; renderer salva sem debounce e responde `app:flushed` → `app.quit()`. Trava de 600 ms força o fechamento.

## Diagnóstico
| Sintoma | Veja |
|---|---|
| CLI sem resposta | daemon fora do ar; confira `$XDG_RUNTIME_DIR/colmeia.sock` e reabra o app |
| Card em branco | `colmeia read <id>` |
| `state.json` mudou sozinho | `loadWorkspace()` em laço; use `allCardIds()` |
