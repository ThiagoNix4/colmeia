# Colmeia no Windows

Windows 10/11 x64. O app empacota tudo que os cards de terminal precisam (UI Electron + daemon `colmeiad.exe` + CLI `colmeia`).

## Para usar (usuário final)

1. Baixe `Colmeia Setup <versão>.exe` (instalador) ou `Colmeia <versão>.exe` (portátil) na página Releases.
2. Execute. O SmartScreen pode avisar "editor desconhecido" (o app não é assinado): **Mais informações → Executar assim mesmo**.
3. Abra **Colmeia** pelo menu Iniciar. Cards **Terminal** já funcionam.

### O que instalar à parte (só se for usar)

| Para quê | O que baixar | Obrigatório? |
|---|---|---|
| Cards de agente (Gerente, Apis Infimus/Medius) | [Claude Code](https://docs.claude.com/claude-code): no PowerShell, `irm https://claude.ai/install.ps1 \| iex`; depois rode `claude` uma vez para fazer login | só para agentes |
| Claude Code no Windows | [Git for Windows](https://git-scm.com/download/win) (o Claude Code usa o Git Bash) | só para agentes |
| Linha de status do Claude dentro do card | [Node.js](https://nodejs.org) 20+ LTS | não (sem ele, só não aparece a linha de status) |
| Shell mais moderno nos cards | [PowerShell 7](https://aka.ms/powershell) (`pwsh`); é usado automaticamente se estiver no PATH | não |
| Agentes opencode | `opencode` no PATH | não |

Depois de instalar algo, **feche e reabra o Colmeia** (os cards herdam o PATH do momento em que o daemon subiu; se o PATH mudou, use "Reiniciar daemon" no aviso ou reinicie o Windows).

A CLI `colmeia` dentro dos cards **não precisa de Node.js**: `cli\colmeia.cmd` usa o `node` do PATH se existir e, senão, o próprio `Colmeia.exe` em modo Node (variável `COLMEIA_NODE`, definida pelo app).

## Como funciona (resumo)

- O app copia `colmeiad.exe`, `cli\` e `docs\` para `%USERPROFILE%\.local\share\colmeia\<versão>-<hash do daemon>\` e sobe o daemon destacado: os cards continuam vivos ao fechar a janela.
- Transporte: named pipe `\\.\pipe\colmeia-<USUÁRIO>` (ACL só do usuário). Override: `COLMEIA_SOCK_PATH`.
- Shell dos cards: `pwsh.exe` se estiver no PATH, senão `powershell.exe`. Override: `COLMEIA_SHELL`.
- Log do daemon: `%USERPROFILE%\.config\colmeia\colmeiad.log`.
- Ao atualizar pelo instalador, o `colmeiad.exe` antigo é encerrado (`build/installer.nsh`) para o novo entrar em uso; os cards do workspace são recriados ao abrir o app. No portátil, o app pergunta se reinicia o daemon quando a versão muda: escolha **Reiniciar daemon**.

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
npm run dist:win   :: gera dist\Colmeia Setup <versão>.exe e dist\Colmeia <versão>.exe
```

Se `npm run dev` disser `Electron uninstall`, rode `node node_modules\electron\install.js` (o download do Electron no postinstall falhou).

Testes: `go vet ./...` e `go build ./...` em `daemon\` (Windows e `set GOOS=linux`); `npx tsc --noEmit`. Os testes de `internal/api` e `internal/term` assumem Unix (`sh`, socket unix) e falham no Windows; rode-os no Linux.

## Correções da versão Windows beta 2

- ConPTY: `conpty.Spawn` resolvia nome relativo (`powershell.exe`) contra o cwd do card em vez do PATH → nenhum card iniciava. Agora o nome é resolvido com `exec.LookPath` (`daemon/internal/term/pty_windows.go`).
- Atualização: a pasta do daemon copiado era reaproveitada por versão, então um instalador novo com a mesma versão continuava usando o daemon antigo. Agora a pasta leva o hash do binário, e o instalador encerra o daemon antigo.
- CLI `colmeia` sem Node.js (fallback para `Colmeia.exe`).
- Erro de spawn agora aparece no card em vez de deixá-lo parado em "starting".
- `npm run build:daemon:win` funciona no CMD/PowerShell (antes exigia bash).
