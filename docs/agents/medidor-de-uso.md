# Medidor de uso

O medidor da toolbar mostra quanto do limite de uso do agente já foi gasto e quando reseta.

## Claude Code
- No start do app, o Colmeia grava `~/.config/colmeia/claude-settings.json` com uma
  `statusLine` apontando para `cli/colmeia-statusline.mjs`.
- Comandos de preset que começam com `claude` recebem `--settings <arquivo>`, então só os
  cards do Colmeia usam essa statusline; o `claude` do terminal normal não muda.
- A cada atualização da statusline, o script lê `rate_limits` (5 h e semana) do stdin e grava
  `~/.config/colmeia/usage/claude.json`, de forma atômica. Sem `rate_limits`, não grava.
- O medidor lê esse arquivo. Sem ele, mostra "abra um agente Claude para medir".

## Notas
- `XDG_CONFIG_HOME` é respeitado.
- Falhas de escrita são ignoradas: o medidor é opcional e nunca derruba o terminal.

## Escopo dos números
- Claude: os % vêm de `rate_limits` da conta (5 h e semanal) — um limite só para Opus, Sonnet e Haiku juntos; o nome do modelo na statusline é só o modelo daquele card.
- opencode: conta todas as respostas do dia, de todos os modelos/provedores somados.
