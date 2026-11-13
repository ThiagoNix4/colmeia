# Economia de token

Cortar desperdício (saída verbosa, tela despejada, contexto redescoberto), não raciocínio.

## Ler
```sh
rg --files | rg -i termo        # achar arquivo
rg -n -C 3 simbolo arquivo      # achar linha com contexto
sed -n '40,90p' arquivo         # só o trecho
wc -l arquivo                   # decidir se vale ler inteiro
```
- Acima de ~200 linhas, leia por trecho. Nunca leia `node_modules/`, `dist/`, `out/`, lockfiles, `.git/`, binários, logs sem filtro.
- Prefira 1 leitura direcionada a 5 pequenas. Guarde caminhos e linhas da primeira leitura.

## Terminais
- `colmeia ask <id> "tarefa" --result` (ou `colmeia result <id>`) devolve só do último `RESUMO:` em diante, em vez da tela.
- `read`/`ask` já saem compactos; `--raw` devolve a tela crua.
- Tarefa longa: `send` + `wait`, leia só ao final. Sem polling.
- Não peça ao worker para colar arquivo; peça `RESUMO:` + caminhos + linhas.

## Prompts
- Contrato em `PLAN*.md`; o prompt do worker tem 2–4 linhas apontando para ele.
- Não anexe arquivo inteiro: caminho + faixa de linhas.

## Agentes
- Reuse worker ocioso (contexto já carregado); `/clear` se a tarefa nova não tem relação.
- Modelo mais barato que resolve: Haiku (simples: criar algo simples, tsc/build) → Sonnet (código) → Opus (delicado + revisão de código não trivial). opencode/`-free` têm limite diário; sem cota, troque de IA.
- Paralelize com arquivos disjuntos; um escreve, outro revisa/testa.

## Respostas
- Comece com `RESUMO:`; cite `arquivo:linha`; diga o que foi validado e o que falta.
- Não recapitule o pedido nem repita o que o usuário já viu.
- Build/`tsc` uma vez, no fim ou na integração.

## Cache de descoberta
O que custou caro (comando que funcionou, armadilha, erro não óbvio) vira nota (`colmeia note`) ou entra no playbook certo em `docs/agents/playbooks/`.

## Links
Ligar card ou nota é permissão e contexto: ligue só o que o worker precisa (`colmeia link`). `note ls` mostra só as notas ligadas, então o worker lê menos.
