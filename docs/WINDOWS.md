# Colmeia no Windows

Windows 10/11 x64. O app empacota tudo que os cards de terminal precisam (UI Electron + daemon `colmeiad.exe` + CLI `colmeia`).

## Para usar (usuário final)

1. Baixe `Colmeia-Setup-<versão>.exe` (instalador) ou `Colmeia-<versão>-portable.exe` (portátil) na página Releases.
2. Execute. O SmartScreen pode avisar "editor desconhecido" (o app não é assinado): **Mais informações → Executar assim mesmo**.
3. Abra **Colmeia** pelo menu Iniciar. Cards **Terminal** já funcionam.

### O que instalar à parte (só se for usar)

| Para quê | O que baixar | Obrigatório? |
|---|---|---|
| Cards de agente (Gerente, Apis Infimus/Medius) | [Claude Code](https://docs.claude.com/claude-code): no PowerShell, `irm https://claude.ai/install.ps1 \| iex`; depois rode `claude` uma vez para fazer login | só para agentes |
| Claude Code no Windows | [Git for Windows](https://git-scm.com/download/win) (o Claude Code usa o Git Bash) | só para agentes |
| Shell mais moderno nos cards | [PowerShell 7](https://aka.ms/powershell) (`pwsh`); é usado automaticamente se estiver no PATH | não |
| Agentes opencode | `npm i -g opencode-ai` (ou outro instalador); precisa estar no PATH | não |

Depois de instalar algo, **feche e reabra o Colmeia** (os cards herdam o PATH do momento em que o daemon subiu; se o PATH mudou, use "Reiniciar daemon" no aviso ou reinicie o Windows).

A CLI `colmeia` e a linha de status do Claude (que também alimenta o medidor de uso) **não precisam de Node.js**: `cli\colmeia.cmd` e `cli\colmeia-statusline.cmd` usam o `node` do PATH se existir e, senão, o próprio `Colmeia.exe` em modo Node (variável `COLMEIA_NODE`, definida pelo app).

## Como funciona (resumo)

- O app copia `colmeiad.exe`, `cli\` e `docs\` para `%USERPROFILE%\.local\share\colmeia\<versão>-<hash do daemon>\` e sobe o daemon destacado: os cards continuam vivos ao fechar a janela.
- Transporte: named pipe `\\.\pipe\colmeia-<USUÁRIO>` (ACL só do usuário). Override: `COLMEIA_SOCK_PATH`.
- Shell dos cards: `pwsh.exe` se estiver no PATH, senão `powershell.exe`. Override: `COLMEIA_SHELL` (precisa ser um PowerShell: os argumentos passados são `-NoLogo`, `-NoExit`, `-Command`).
- Política de execução: os cards recebem `PSExecutionPolicyPreference=RemoteSigned` (escopo do processo, não muda o sistema), para que CLIs instaladas pelo npm (`opencode`, `codex`, `gemini`…), que são scripts `.ps1`, rodem. Se a variável já estiver definida no ambiente, o valor é mantido.
- Log do daemon: `%USERPROFILE%\.config\colmeia\colmeiad.log`.
- Ao atualizar pelo instalador, o `colmeiad.exe` antigo é encerrado (`build/installer.nsh`: primeiro `--stop`, que grava o histórico dos cards; `taskkill` só de reserva) para o novo entrar em uso; os cards do workspace são recriados ao abrir o app. No portátil, o app pergunta se reinicia o daemon quando a versão muda: escolha **Reiniciar daemon**.
- Portátil sem Node.js instalado: a CLI `colmeia` dentro dos cards usa o executável extraído pelo portátil, que só existe enquanto ele está aberto. Para agentes que continuam trabalhando com o app fechado, use o instalador (ou instale o Node.js).

## Solução de problemas

- **Card não inicia / mostra `[colmeia] falha ao iniciar: …`**: a mensagem diz o motivo. Veja também o log do daemon.
- **Verificar o daemon:** `tasklist | findstr colmeiad` e, no PowerShell, `Get-ChildItem \\.\pipe\ | ? Name -like 'colmeia*'`.
- **Daemon travado ou antigo:** feche o app, rode `taskkill /F /IM colmeiad.exe` e abra o app de novo.
- **Agente fecha na hora:** rode `claude` num card Terminal para ver o erro (login, Git for Windows ausente etc.).

## Compilar no Windows (desenvolvedor)

Pré-requisitos: [Git](https://git-scm.com/download/win), [Node.js](https://nodejs.org) 20+, [Go](https://go.dev/dl/) 1.22+. Com winget:

```bat
winget install -e --id Git.Git
winget install -e --id OpenJS.NodeJS.LTS
winget install -e --id GoLang.Go
```

Depois, num terminal novo (CMD ou PowerShell):

```bat
git clone https://github.com/ThiagoNix4/colmeia
cd colmeia
npm install
npm run dev        :: desenvolvimento (compila o daemon antes: npm run build:daemon:win)
npm run dist:win   :: gera dist\Colmeia-Setup-<versão>.exe e dist\Colmeia-<versão>-portable.exe
```

Release com versão própria (sem mudar o `package.json`): `npx electron-vite build`, `npm run build:daemon:win` e `npx electron-builder --win portable nsis --x64 --config.extraMetadata.version=0.1.0-win.N`.

Se `npm run dev` disser `Electron uninstall`, rode `node node_modules\electron\install.js` (o download do Electron no postinstall falhou).

Testes: `go vet ./...` e `go build ./...` em `daemon\` (Windows e `set GOOS=linux`); `go test ./...` (passa no Windows e no Linux; no Windows os testes de `internal/term` rodam o PowerShell via ConPTY e os de `internal/api` usam named pipe); `npx tsc --noEmit`.

## Correções da versão Windows beta 3 (v0.1.0-win.3)

- **opencode (e outras CLIs do npm) não abriam:** o PowerShell recusava o shim `opencode.ps1` ("a execução de scripts foi desabilitada"). Os cards agora rodam com `PSExecutionPolicyPreference=RemoteSigned` (`daemon/internal/api/spawn.go`).
- **Letra apagada continuava na tela (Windows 10):** com a página de código UTF-8 (Claude Code, opencode), o conhost do Windows 10 devolve bytes de controle como `SI` (0x0F) como se fossem uma célula; o terminal ignorava o byte e a linha ficava uma coluna desalinhada, então o backspace apagava a célula errada até a linha ser repintada. O daemon troca esses bytes por espaço (`blankControlCells` em `daemon/internal/term/pty_windows.go`; só em builds anteriores ao Windows 11).
- **`colmeia send` de texto longo chegava cortado (Windows 10):** o conhost descarta os marcadores de bracketed paste e o Claude Code perdia o começo do texto. O envio agora é feito em pedaços de 256 bytes espaçados (`daemon/internal/term/paced_windows.go`).
- **CLI `colmeia` devolvia código de saída errado:** `%errorlevel%` dentro de bloco `( )` no `colmeia.cmd` (sempre 0 com Node, sempre 1 sem). Corrigido; `colmeia wait` volta a sinalizar timeout.
- Instalador: para o daemon com `--stop` antes do `taskkill` (não perde os últimos segundos de histórico) e o desinstalador encerra o daemon e remove `%USERPROFILE%.localsharecolmeia`.
- Notas: o vault do Obsidian é detectado em `%APPDATA%obsidianobsidian.json`.
- UI: cabeçalho do card e da nota mostram só o nome da pasta/arquivo; a pasta do workspace é abreviada com `~`.
- Cópias antigas do daemon em `.localsharecolmeia` são removidas ao atualizar (exceto a de um daemon ainda em uso).
- Workspaces e notas com nome reservado do Windows (`con`, `nul`, `com1`…) ganham o prefixo `_`; `colmeia send -` aceita CRLF; aspas simples no caminho do perfil não quebram mais o preset Gerente.

## Correções da versão Windows beta 2 (v0.1.0-win.2)

- ConPTY: `conpty.Spawn` resolvia nome relativo (`powershell.exe`) contra o cwd do card em vez do PATH → nenhum card iniciava. Agora o nome é resolvido com `exec.LookPath` (`daemon/internal/term/pty_windows.go`).
- Atualização: a pasta do daemon copiado era reaproveitada por versão, então um instalador novo com a mesma versão continuava usando o daemon antigo. Agora a pasta leva o hash do binário, e o instalador encerra o daemon antigo.
- CLI `colmeia` e linha de status do Claude sem Node.js (fallback para `Colmeia.exe`).
- Erro de spawn agora aparece no card em vez de deixá-lo parado em "starting".
- `npm run build:daemon:win` funciona no CMD/PowerShell (antes exigia bash).
- Testes Go passam no Windows (ConPTY + named pipe).
- Atualizar pelo instalador reinicia os terminais (o daemon antigo é encerrado); o workspace e os cards são mantidos.
