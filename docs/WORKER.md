# Regras do worker (Colmeia)

Você é um worker num card do Colmeia. Faça só a sua parte do contrato (`docs/dev/plan*.md`).

## Ler pouco
- Ache com `rg -n`, leia com `sed -n 'A,Bp'`. `cat` só em arquivo curto.
- Nunca ler `node_modules/`, `out/`, `dist/`, `build/`, `.git/`, lockfiles, logs sem filtro.
- Programe contra o contrato do PLAN, não contra o estado atual do disco (outros workers escrevem em paralelo).
- Não releia o que já leu; use `git diff -- arquivo` para conferir o que editou.

## Escopo
- Edite só os arquivos que o PLAN atribui a você. Um arquivo, um dono.
- Confira com `npx tsc --noEmit -p tsconfig.json` (CLI: `node --check cli/colmeia.mjs`).
- Não rode `npm run build`, não reinicie/feche o app Colmeia, não faça commit, não instale dependência.
- Estilo: TypeScript estrito, comentários curtos em português, escrita atômica (`.tmp` + `rename`).

## Saída econômica
- Fale com o gerente só em inglês. Ações, não texto: responda `OK`, `RESUMO:` curto ou nada, conforme o prompt.
- Sem preâmbulo, sem recapitular a tarefa, sem narrar cada passo.
- Não cole de volta o código que escreveu; cite `arquivo:linha`.
- Termine com `RESUMO:` de até ~3 linhas (ou o limite que o prompt pedir; se pedir sem resposta, responda só `OK`).
- Testes: no máximo 3, só os essenciais.
- Pense o quanto precisar: a economia é na saída, não no raciocínio.

## Links e notas
- Link no canvas = permissão: você só comanda cards e lê/escreve notas ligados a você.
- Ao começar, rode `colmeia note ls`: lê só as notas listadas (as ligadas a você). Nenhuma listada = não leia nota.
- Ao escrever em nota: inglês telegráfico (sem artigos, `chave: valor`, símbolos `→ ✓ ✗`).
- Sem link, o comando falha com `sem link: ...`: avise o gerente em vez de insistir.
