# Colmeia

Um canvas infinito onde cada card é um terminal real. Você distribui as IAs (Claude Code, opencode, shells) pelo espaço, vê todas trabalhando ao mesmo tempo e manda mensagens para uma, para várias ou para todas. Um card **Gerente** (Claude) consegue criar e comandar os outros cards sozinho pela CLI `colmeia`.

Feito para Linux (testado no Arch, Wayland/GNOME), com versão beta para Windows. Usa Electron (UI) + daemon Go `colmeiad` (PTYs) + xterm.js + React Flow.

## Requisitos
- Node.js 20+ e npm. O `node` do sistema é dependência de runtime: a CLI `colmeia` e o statusline dos cards rodam com ele (o pacote `.deb` declara `depends: nodejs`; no AppImage instale o Node à parte). No Linux não há fallback para o executável do app (no Windows há; veja abaixo).
- Go 1.22+ (compila o daemon `colmeiad`)
- Opcional: `claude` (Claude Code) e `opencode` no PATH

## Novidades da 0.1.2
- Cards do Claude e do opencode reabrem na **mesma conversa** (sessão fixa por card); o opencode não fica mais com a tela quebrada ao reabrir o app.
- opencode recebe as instruções do Colmeia como prompt de sistema: nenhuma mensagem é enviada ao abrir o card (sem gastar tokens).
- Windows: CLIs instaladas com o app aberto (ex.: `claude`) funcionam nos cards novos, e não piscam mais janelas de console ao abrir o app.
- Clique e seleção no terminal acertam a célula com qualquer zoom do canvas.
- Variantes (forks) do Colmeia rodam lado a lado sem misturar daemon e dados (ver [Variantes](#variantes-forks-do-colmeia)).
- Nota `Handoff` nas diretrizes do Gerente e dos workers para passar trabalho entre cards.

## Novidades da 0.1.1
- Gerente escolhe entre todos os agentes do PC (`colmeia agents`), policy por workspace free/ask/all (`colmeia policy` + seletor Modelos na barra).

## Download (Releases)
Baixe na página **Releases** do GitHub. O código é o mesmo para Linux e Windows; só muda o pacote.

**Linux x64** (pacotes na release 0.1.1; para a 0.1.2 no Linux, instale a partir do código)
- **AppImage:** `chmod +x Colmeia-0.1.1.AppImage && ./Colmeia-0.1.1.AppImage`
- **Debian/Ubuntu:** `sudo apt install ./colmeia_0.1.1_amd64.deb`

**Windows 10/11 x64 (beta)**
- **Instalador:** baixe `Colmeia-Setup-0.1.2.exe`, execute e abra **Colmeia** pelo menu Iniciar.
- **Portátil:** baixe `Colmeia-0.1.2-portable.exe` e abra com duplo clique (não precisa instalar).
- O SmartScreen pode avisar "editor desconhecido" (o app não é assinado): clique em **Mais informações → Executar assim mesmo**.
- **Não precisa instalar nada além do app** para os cards de terminal. A CLI `colmeia` e a linha de status do Claude usam o Node.js se estiver no PATH; senão usam o próprio `Colmeia.exe` como Node.
- Para os cards de agente (Gerente, Apis…): instale o [Claude Code](https://docs.claude.com/claude-code) (`irm https://claude.ai/install.ps1 | iex` no PowerShell) e faça login uma vez com `claude`. Ele exige o [Git for Windows](https://git-scm.com/download/win).
- Opcional: [PowerShell 7](https://aka.ms/powershell) (`pwsh`, usado no lugar do Windows PowerShell se instalado; troque com `COLMEIA_SHELL`, que precisa ser um PowerShell), `opencode` e outras CLIs de agente instaladas pelo npm.
- Ao atualizar, o instalador fecha o daemon antigo; os cards do workspace são recriados ao abrir o app. Guia completo e solução de problemas: [docs/WINDOWS.md](docs/WINDOWS.md).

Nos dois sistemas o app sobe o daemon `colmeiad` sozinho, e os cards continuam vivos ao fechar a janela.

## Instalação a partir do código (Linux)
```sh
cd ~/colmeia
npm install            # instala as dependências
npm run install-app    # build + CLI `colmeia` em ~/.local/bin + atalho no menu
```
Depois disso, abra **Colmeia** pelo menu do GNOME. Para desenvolver, use `npm run dev`.

## Instalação a partir do código (Windows)
Requer Node.js 20+, Go 1.22+ e Git. No PowerShell, dentro da pasta do projeto:
```powershell
npm ci                 # dependências
npm run dist:win       # gera dist\Colmeia-Setup-<versão>.exe e o portátil
```
Para desenvolver: `npm run build:daemon:win` uma vez e depois `npm run dev`.

CLIs instaladas com o app já aberto (ex.: `claude`) aparecem nos cards novos sem reiniciar nada: o daemon relê o PATH do registro do Windows a cada card.

Para gerar os pacotes: `npm run dist` (Linux: AppImage + .deb) e `npm run dist:win` (Windows: instalador + portátil; no Windows roda direto com Node + Go, a partir do Linux precisa também de `wine`). Veja [docs/WINDOWS.md](docs/WINDOWS.md).

## Uso
| Ação | Como |
|---|---|
| Novo card | Botões coloridos na barra superior (um por preset) |
| Pasta dos novos cards | Botão 📁 na barra superior |
| Mover o canvas | Roda do mouse, ou arrastar com o botão do meio/direito |
| Zoom | Ctrl + roda |
| Selecionar vários | Arrastar no fundo com o botão esquerdo, ou Ctrl/Shift + clique |
| Mover/redimensionar card | Arrastar pelo cabeçalho / pelas bordas do card selecionado |
| Renomear | Duplo clique no título |
| Focar (zoom 100%) | Botão ⌖, ou duplo clique no cabeçalho |
| Conectar cards | Arrastar da bolinha direita de um card até a esquerda de outro |
| Copiar / colar no terminal | Ctrl+Shift+C / Ctrl+Shift+V |
| Mensagem em massa | Barra inferior: alvo **Selecionados**, **Conectados aos selecionados** ou **Todos**. Enter envia, Shift+Enter quebra linha |
| Workspaces | Seletor no canto superior direito: ＋ cria, ✎ renomeia e 🗑 exclui o workspace atual |

O layout é salvo sozinho. Fechar o app não fecha os terminais: o daemon os mantém. Ao reabrir:
- **Claude Code e terminais comuns** continuam exatamente onde estavam.
- **opencode** é reaberto na **mesma conversa** (cada card tem uma sessão fixa, `--session ses_colmeia_<id>`), desenhado do zero. A tela cheia do opencode não sobrevive a um replay de bytes.
- Se o PC reiniciou, cada card do Claude volta na sua conversa (`--resume`, com uma sessão fixa por card) e o do opencode também.

Para passar trabalho a um card novo, o Gerente e os workers registram o que fizeram na nota `Handoff` (regras em `docs/ORCHESTRATOR.md` e `docs/WORKER.md`).

As instruções do Colmeia entram de forma implícita: no Claude Code por `--append-system-prompt`, no opencode pelo campo `instructions` da configuração. Nenhuma mensagem é enviada ao abrir o card, então isso não gasta tokens.

## CLI `colmeia`
Funciona dentro de qualquer card (já está no PATH deles) e em qualquer terminal depois do `install-app`.

```
colmeia list                                  lista os cards
colmeia presets                               lista os presets disponíveis
colmeia agents                                inventário de agentes do PC (tool, modelo, custo) + policy
colmeia policy [free|ask|all]                 mostra / define a policy de modelos do workspace
colmeia spawn <preset> [--title T] [--cwd D] [--near ID]
                                             cria um card (imprime o id)
colmeia send <id> <texto...> [--no-enter]     escreve no card e aperta Enter
colmeia key <id> <ctrl-c|esc|enter|tab|up|down>  envia uma tecla
colmeia broadcast <texto...> [--ids a,b]      envia para vários (padrão: todos menos você)
colmeia read <id> [--lines N]                 últimas N linhas (padrão 60), sem cores
colmeia wait <id> [--idle 5] [--timeout 600]  espera o card ficar ocioso (segundos)
colmeia ask <id> <texto...> [--idle 8]        send + wait + read numa tacada só
colmeia link <a> <b>                          conecta dois cards no canvas
colmeia focus <id>                            centraliza o card na tela
colmeia kill <id>                             fecha o card
colmeia screenshot [arquivo.png]              captura a janela do canvas
colmeia whoami                                id do card atual
```
O `<id>` aceita o id, um prefixo dele ou o título do card. Use `-` como texto para ler do stdin.

## O Gerente
O preset **Gerente** abre o Claude Code com as instruções de `docs/ORCHESTRATOR.md`. Ele age como a rainha da colmeia: não põe a mão no código, só planeja e distribui por castas — **Batedor** (Haiku, mapeia o repo), **Operária** (Sonnet, implementa), **Verificador** (Haiku, tsc/build/testes), **Revisor** (Opus, lê o diff) — escolhendo sempre o modelo mais barato que resolve e lendo só o `RESUMO:` de cada um. Os workers aparecem ao lado dele no canvas, ligados por uma linha.

## Configuração
- Presets: `~/.config/colmeia/presets.json`
- Workspaces: `~/.config/colmeia/workspaces/*.json`
- Pasta padrão dos projetos: `~/ai-projects`
- Socket da API: `$XDG_RUNTIME_DIR/colmeia.sock` (Windows: named pipe `\\.\pipe\colmeia-<usuário>`)

No Windows (inclusive `C:\Users\<você>\.config\colmeia`), os caminhos são os mesmos, a partir da pasta do usuário.

## Variantes (forks) do Colmeia
Cada instalação tem uma **instância**, que é o `"name"` do `package.json` (padrão `colmeia`). Dela saem a pasta de config (`~/.config/<name>`), o socket/pipe do daemon, a pasta de dados (`~/.local/share/<name>`), o perfil do Electron e a trava de "um app aberto por vez".

Para criar uma variante que rode **ao lado** do Colmeia original, sem misturar workspaces, agentes e cards, mude no `package.json`:
```json
"name": "colmeia-minha-variante",
"build": { "appId": "dev.colmeia.minha-variante", "productName": "Colmeia Minha Variante" }
```
Se o `name` ficar igual, as duas cópias são o **mesmo app**: dividem o daemon e os dados. Para testar sem editar o arquivo, também dá para usar a variável `COLMEIA_INSTANCE=<nome>`. Um app aberto de dentro de um card (por exemplo, uma IA rodando `npm run dev`) ignora as variáveis herdadas do card.

Exemplo de preset novo (os presets são lidos quando o app abre):
```json
{ "id": "claude-sonnet", "label": "Sonnet", "command": "claude --model sonnet", "color": "#9b7bf0", "keepShell": true }
```
- `command`: roda num shell de login. Vazio abre um shell puro. `{docs}` é trocado pela pasta `docs/` do app.
- `keepShell`: quando o comando termina, o card continua aberto num shell.

## Agentes e economia de token

Regras dos workers em [`docs/WORKER.md`](docs/WORKER.md); referência do CLI, notas, arquitetura, protocolo de economia e playbooks em [`docs/agents/`](docs/agents/). O Gerente usa `colmeia ask --result` / `colmeia result` para ler só o `RESUMO:` dos workers.

## Notas
- Os terminais usam o renderer DOM do xterm com a fonte **FiraCode Nerd Font Mono** (alias `CanvasTerm` em `src/styles.css`), porque o renderer WebGL desenhava a Nerd Font errado. Para testar o WebGL, rode `localStorage.setItem('colmeia.webgl','1')` no DevTools (Ctrl+Shift+I) e recarregue.
- Se `colmeia` não for encontrado fora do app, adicione `export PATH="$HOME/.local/bin:$PATH"` ao `~/.zshrc`. Dentro dos cards ele já está no PATH.
