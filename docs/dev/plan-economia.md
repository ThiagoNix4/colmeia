# PLAN-ECONOMIA — gastar menos token sem perder qualidade

Princípio: **não** cortar raciocínio nem contexto necessário. Cortar desperdício:
saída verbosa, telas de terminal despejadas inteiras, especificação repetida em prompt,
contexto redescoberto. E tudo que o Colmeia precisa fica **dentro do repositório** (vai pro
GitHub), nada em config pessoal (`~/.claude`, vault do Obsidian).

## Workers e arquivos (disjuntos)
| Worker | Modelo | Arquivos |
|---|---|---|
| **CLI** | Haiku | `cli/colmeia.mjs` |
| **Docs** | Sonnet | `docs/WORKER.md` (novo), `docs/ORCHESTRATOR.md`, `docs/agents/**` (novo), `README.md` (só uma seção curta) |
| **UI** | Sonnet | `src/components/AgentsMenu.tsx` |

### CLI — leitura compacta (maior economia do gerente)
- `colmeia read` e `colmeia ask` passam a imprimir **compacto por padrão**:
  remove linhas vazias repetidas (máx. 1 seguida), linhas só de moldura/box-drawing
  (`─━│┃╹▀▄█` e afins, ou linhas sem letra/dígito), espaços à direita, e rodapés de TUI
  conhecidos (linhas com `auto mode on`, `shift+tab to cycle`, `esc to interrupt`,
  `ctrl+p commands`, `? for shortcuts`, `Tip:`). `--raw` devolve o comportamento antigo.
- Novo `colmeia result <id> [--lines N]`: lê a tela (padrão 200 linhas) e imprime só do
  **último `RESUMO:`** em diante, compacto; se não houver `RESUMO:`, imprime as últimas 15
  linhas compactas e avisa na stderr.
- Novo flag `ask ... --result`: em vez da tela, imprime como `result`.
- Padrão de `read` cai de 60 para 40 linhas; `ask` de 80 para 40.
- Atualizar o texto de ajuda. Sem dependência nova.

### Docs — regras no repo
- `docs/WORKER.md` (novo, **curto**, ≤ 40 linhas): regras de todo worker — ler pouco
  (`rg -n`, `sed -n`), não ler `node_modules/out/dist/lock`, programar contra o PLAN,
  só mexer nos seus arquivos, `tsc` para conferir, não build/não reiniciar/não commitar, e
  **saída econômica**: sem preâmbulo, sem recapitular a tarefa, sem colar código escrito,
  sem narrar cada passo; terminar com `RESUMO:` de até ~8 linhas (arquivo → mudança, desvios,
  pendências). Pensar o quanto precisar — economia é na saída, não no raciocínio.
- `docs/ORCHESTRATOR.md`: acrescentar seção **Economia de token** (curta): usar
  `colmeia ask --result`/`colmeia result` em vez de `read`; contrato em `PLAN*.md` e prompt
  de worker de 2–4 linhas apontando pro arquivo; reusar workers (contexto já carregado) e
  `/clear` quando a tarefa nova não tem relação; modelo mais barato que resolve; respostas ao
  usuário diretas, sem repetir o que ele já viu; rodar `tsc`/build uma vez no fim.
  Trocar referências ao vault (`<vault>/...`) por `docs/agents/`. Não pode
  haver caminho pessoal (`/home/<user>`) em nada do repo que você tocar.
- `docs/agents/`: trazer para o repo, **genéricos** (sem caminhos pessoais), o conteúdo útil de:
  `Protocolo de Economia de Token.md`, `Colmeia - Referencia do CLI.md`,
  `Notas por workspace.md` e `Playbooks/*.md` que estão em
  `"<vault>/Agentes/"` (e `Projetos/Colmeia - Arquitetura.md`
  se existir). Enxugar ao condensar; nomes de arquivo em kebab-case. O vault continua
  existindo; a fonte de verdade do app passa a ser o repo.
- `README.md`: seção curta "Agentes e economia de token" apontando para `docs/`.

### UI — regras do Colmeia nos agentes
- No formulário de agente, checkbox **"Regras do Colmeia (economia de token)"**, padrão ligado,
  só para ferramenta Claude Code: acrescenta ao comando
  `--append-system-prompt "$(cat '{docs}/WORKER.md')"` (o `{docs}` já é resolvido pelo
  ptyManager). Ao reabrir um agente, o checkbox reflete se o comando contém `{docs}/WORKER.md`.
  Para as outras ferramentas, não mostrar o checkbox.

## Regras
Ler `colmeia note read 'Regras dos Agentes'`. Só seus arquivos. Pronto: `npx tsc --noEmit -p
tsconfig.json` limpo (CLI: `node cli/colmeia.mjs --help` roda). Não build, não reiniciar, não
commitar. Fim com `RESUMO:` curto.
