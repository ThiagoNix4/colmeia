# Colmeia

Um canvas infinito onde cada card é um terminal real. Você distribui as IAs (Claude Code, opencode, shells) pelo espaço, vê todas trabalhando ao mesmo tempo e manda mensagens para uma, para várias ou para todas. Um card **Gerente** (Claude) consegue criar e comandar os outros cards sozinho pela CLI `colmeia`.

Feito para Linux (testado no Arch, Wayland/GNOME), com versão beta para Windows. Usa Electron (UI) + daemon Go `colmeiad` (PTYs) + xterm.js + React Flow.

## Requisitos
- Node.js 20+ e npm. O `node` do sistema é dependência de runtime: a CLI `colmeia` e o statusline dos cards rodam com ele (o pacote `.deb` declara `depends: nodejs`; no AppImage instale o Node à parte). No Linux não há fallback para o executável do app (no Windows há; veja abaixo).
- Go 1.22+ (compila o daemon `colmeiad`)
- Opcional: `claude` (Claude Code) e `opencode` no PATH

## Novidades da 0.1.1
- Gerente escolhe entre todos os agentes do PC (`colmeia agents`), policy por workspace free/ask/all (`colmeia policy` + seletor Modelos na barra).

## Download (Releases)
Baixe na página **Releases** do GitHub. O código é o mesmo para Linux e Windows; só muda o pacote.

**Linux x64**
- **AppImage:** `chmod +x Colmeia-0.1.1.AppImage && ./Colmeia-0.1.1.AppImage`
- **Debian/Ubuntu:** `sudo apt install ./colmeia_0.1.1_amd64.deb`

**Windows 10/11 x64 (beta)**
- **Instalador:** baixe `Colmeia-Setup-0.1.1.exe`, execute e abra **Colmeia** pelo menu Iniciar.
- **Portátil:** baixe `Colmeia-0.1.1-portable.exe` e abra com duplo clique (não precisa instalar).
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

O layout é salvo sozinho. Ao reabrir o app, os cards voltam e os comandos deles são executados de novo (o histórico do terminal não é restaurado).

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
- Socket da API: `$XDG_RUNTIME_DIR/colmeia.sock`

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
