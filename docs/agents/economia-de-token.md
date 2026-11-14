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

## Onde vão os tokens (maior → menor)
1. Contexto do Gerente: relido a cada turno → Gerente não lê código nem tela; só `RESUMO:`.
2. Card novo: cada Claude carrega milhares de tokens de prompt de sistema → reuse ocioso, `/clear` em vez de kill+spawn.
3. Saída (~5× o preço da entrada) → worker sem explicação, formato fixo por papel.
4. Leitura dos workers → `rg`+`sed`, nada de `out/`/`node_modules/`.
5. Retrabalho → contrato no PLAN; revisor vê só o diff; 2 falhas → escala modelo.
6. Polling (`read` em loop), imagens, build repetido.

## Agentes (castas)
- Gerente planeja/distribui/integra, não codifica. Batedor (Haiku) mapeia o repo. Operária (Sonnet; Haiku se mecânica) implementa. Verificador (Haiku) roda tsc/build/testes. Revisor (Opus) lê o diff. Especialista (Opus) só após 2 falhas ou bug sem pista. Detalhe em `docs/ORCHESTRATOR.md` e `docs/WORKER.md`.
- Reuse worker ocioso (contexto já carregado); `/clear` se a tarefa nova não tem relação.
- Escada: Haiku → Sonnet → Opus; sobe após 2 falhas. opencode/`-free` têm limite diário; sem cota, troque de IA.
- Paralelize com arquivos disjuntos; um escreve, outro revisa/testa.

## Respostas
- Comece com `RESUMO:`; cite `arquivo:linha`; diga o que foi validado e o que falta.
- Não recapitule o pedido nem repita o que o usuário já viu.
- Build/`tsc` uma vez, no fim ou na integração.

## Cache de descoberta
O que custou caro (comando que funcionou, armadilha, erro não óbvio) vira nota (`colmeia note`) ou entra no playbook certo em `docs/agents/playbooks/`.

## Links
Ligar card ou nota é permissão e contexto: ligue só o que o worker precisa (`colmeia link`). `note ls` mostra só as notas ligadas, então o worker lê menos.
