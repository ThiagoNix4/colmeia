## Novidades

- **`colmeia wait`** aceita vários ids (`--all` / `--any`) e informa ok, erro, saiu ou timeout, com a última linha de saída.
- **`spawn --model`** e novos comandos `colmeia preset new` / `preset rm`; `spawn` aceita o título do preset.
- **Agentes padrão:** apenas Gerente e Revisor, ambos editáveis, com "restaurar padrão".
- **Layout automático** de cards e notas em grade, e comando `colmeia tidy` para reorganizar.
- **Modo de ligação:** botão direito no card → "Criar link", depois clique nos cards para ligar (Esc sai).
- **Notas iniciais** criadas em workspaces novos (Contexto, Comandos, Decisões, Handoff).
- **CLI:** stdin seguro com BOM, `result` mostra só o RESUMO, `colmeia --version`, `agents set <modelo> free|paid|auto`.
- **Windows:** PATH com npm e opencode, cards de shell em UTF-8, correção de resposta de terminal que vazava no `send` logo após o `spawn`.
- **Gerente:** delega tudo a cards do Colmeia, economiza tokens e responde no idioma do usuário.
- Novo playbook de diagnóstico do sistema Windows e workflow de release para Windows.
