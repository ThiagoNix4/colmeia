# Playbook — tarefas de sistema Windows (fora do código)

Para pedidos que mexem no PC/sistema (permissões, rede, serviços, drivers, agendamento, limpeza, apps instalados), não em código do repo.

## Ordem
1. **Diagnóstico read-only.** `Get-*`, `Get-ChildItem`, `reg query`, `tasklist`, `sc query`, logs/eventos. Nada de `Set-*`, `Remove-*`, `reg add/delete`, `Stop-*` nesta fase.
2. **Aprovação.** Mostre ao usuário: sintoma, causa provável, plano de correção, risco e reversão. Só execute depois do OK explícito.
3. **Correção mínima.** Menor mudança que resolve; backup do que for tocar (export de chave, atalho, arquivo) antes. Preferível reverter que descrever.
4. **Verificação.** Rode de novo o diagnóstico/comando que mostrou o problema e mostre antes/depois. Se não resolver, volte ao passo 1 (não tente 2 golpes diferentes).

## Limites de admin
- Card pode não ter admin. O que mexe em `HKLM`, arquivos de sistema, serviços, firewall, drivers ou UAC precisa de elevação.
- Sem admin: diga o que precisa e peça para o usuário rodar como administrador ou aprovar o UAC. Não tente burlar.
- Aprovação do usuário cobre a ação, não a elevação: confirme antes de lançar o prompt de UAC.

## Sem projeto
- Tarefa de sistema não tem pasta de projeto: plano e achados em `docs/dev/` do workspace atual (nunca `~`).
- Notas: as do workspace e este playbook.

## Segurança
- Proibido sem aprovação explícita: `Remove-Item -Recurse -Force`, `Format-Volume`, `diskpart`, `Clear-*`, `reg delete`, `Stop-Computer`, desinstalador.
- Mudança de sistema relevante → crie ponto de restauração antes.
- Não desligue/reinicie sem pedir.

## CLI
- Diagnóstico longo: `colmeia wait <id...> --all` (uma linha por card: `ok`/`ERRO`/`saiu`/`timeout`).
- Envio de script/texto longo: `colmeia send <id> -` (stdin BOM-safe).

## Idioma
- Usuário: no idioma da última mensagem dele. Worker ↔ Gerente e notas: inglês.
