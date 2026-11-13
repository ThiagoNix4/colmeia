# plan-uso — medidor de uso por agente de IA

Um medidor compacto na toolbar mostra **um** provedor por vez (o usuário escolhe qual).
Mostra quanto já usou do limite e quando reseta, para ver que está chegando no limite.

## Fontes de dados (já investigadas — não redescobrir)
- **Claude Code**: o JSON que o Claude Code manda para o comando de `statusLine` traz
  ```json
  "rate_limits": { "five_hour": {"used_percentage": 34, "resets_at": 1794532200},
                   "seven_day": {"used_percentage": 7,  "resets_at": 1794963600} },
  "model": {...}, "cost": {"total_cost_usd": ...}
  ```
  (`resets_at` em segundos epoch). O Colmeia captura isso injetando `--settings <arquivo>`
  em todo card cujo comando começa com `claude`.
- **opencode**: SQLite em `~/.local/share/opencode/opencode.db` (abrir **read-only**; WAL).
  Tabela `session_message(id, session_id, type, seq, time_created, time_updated, data)`;
  `data` é JSON. Mensagem de assistente tem `"model":{"id","providerID"}` e `time.created` (ms).
  Limite estourado aparece como `"error":{"type":"provider.free-limit", ..., "retryAfterMs": N}`
  (com `time.created`) → reset = created + retryAfterMs. Não existe número oficial de limite;
  o usuário pode definir um limite diário de mensagens (opcional).
- Outros (codex, gemini): fora do escopo agora; o formato abaixo já comporta.

## Contrato (`electron/types.ts`)
```ts
export interface UsageWindow {
  label: string          // "5 horas", "Semana", "Hoje"
  pct: number | null     // 0..100; null se não há limite conhecido
  used?: number          // ex.: mensagens hoje
  limit?: number         // limite definido pelo usuário
  resetsAt?: number      // epoch ms
}
export interface UsageProvider {
  id: 'claude' | 'opencode'
  label: string
  status: 'ok' | 'warn' | 'exhausted' | 'unknown'   // warn >= 70%, exhausted >= 100% ou erro de limite ativo
  windows: UsageWindow[]
  detail?: string        // texto curto (modelo, "limite atingido até 21:01", "sem dados ainda")
  updatedAt?: number     // epoch ms do dado
}
export interface UsageReport { providers: UsageProvider[] }
export interface UsageSettings { opencodeDailyLimit?: number }
```
IPC: `usage:get` → `UsageReport`; `usage:setSettings(UsageSettings)` → `UsageReport`.
Preload: `window.canvas.usage.get()`, `window.canvas.usage.setSettings(s)`.
Arquivos de dados (gerados pelo app, fora do repo): `~/.config/colmeia/usage/claude.json`,
`~/.config/colmeia/usage-settings.json`, `~/.config/colmeia/claude-settings.json`.

## Workers (arquivos disjuntos)
| Worker | Modelo | Arquivos |
|---|---|---|
| **Uso-Main** | Sonnet | `electron/usage.ts` (novo), `electron/types.ts`, `electron/main.ts`, `electron/preload.ts` |
| **Statusline** | Haiku | `cli/colmeia-statusline.mjs` (novo), `electron/ptyManager.ts`, `docs/agents/medidor-de-uso.md` (novo) |
| **Medidor-UI** | Sonnet | `src/components/UsageMeter.tsx` (novo), `src/components/Toolbar.tsx`, `src/styles.css` |

### Statusline
- `cli/colmeia-statusline.mjs`: lê o JSON do stdin; grava (atômico, `.tmp`+`rename`) em
  `~/.config/colmeia/usage/claude.json`: `{ rate_limits, model: model.display_name ?? model.id,
  cost_usd, updatedAt: Date.now() }`; só grava se `rate_limits` existir. Imprime 1 linha curta
  no stdout (vira a statusline do card): `<modelo> · 5h 34% · sem 7%`. Nunca lança (try/catch,
  exit 0). Respeita `XDG_CONFIG_HOME`.
- `ptyManager.ts`: gerar (atômico, a cada start do app) `~/.config/colmeia/claude-settings.json`
  = `{"statusLine":{"type":"command","command":"node '<cliDir>/colmeia-statusline.mjs'"}}`
  (`cliDir` = mesmo resolvido em `electron/store.ts`, exportado `cliDir()`). Se o comando do
  preset começa com `claude` (primeira palavra) e ainda não tem `--settings`, inserir
  ` --settings '<arquivo>'` logo após `claude`.
- `docs/agents/medidor-de-uso.md`: como funciona, curto.

### Uso-Main
- `electron/usage.ts`: `getReport(): UsageReport`, `setSettings(s)`.
  - claude: lê `usage/claude.json`; janelas "5 horas" e "Semana" com pct/resetsAt (s→ms).
    Sem arquivo → status `unknown`, detail "abra um agente Claude para medir".
    Se `resetsAt` já passou, pct da janela = 0.
  - opencode: `node:sqlite` (`DatabaseSync`, `readOnly: true`), consulta só mensagens de hoje
    (time_created >= meia-noite local) e o último erro `provider.free-limit` (LIKE no data,
    ordenado por time_created desc, limit 1). Janela "Hoje": used = nº de respostas do
    assistente hoje, limit = `opencodeDailyLimit` se definido (pct = used/limit) senão pct null.
    Erro de limite com reset no futuro → status `exhausted`, detail "limite atingido até HH:MM",
    resetsAt. Sem banco → `unknown`. Tudo em try/catch; nunca derruba o app. Cache de 15 s.
- `main.ts`: handlers `usage:get` e `usage:setSettings`. `preload.ts`: `usage.get/setSettings`.

### Medidor-UI
- `UsageMeter.tsx` na toolbar, à direita, antes do botão de pasta. Fechado: um pill pequeno
  com bolinha de status + nome do provedor + mini barra (da janela mais crítica) + `34%`
  (ou "esgotado"/"—"). Clique abre popover com: seletor do provedor (lista dos provedores, o
  ativo marcado) e os detalhes **só do provedor selecionado** (cada janela: barra, %, "reseta
  às HH:MM" ou "em 2h 10m", used/limit quando houver; para opencode um campo "limite diário
  de mensagens" que chama `usage.setSettings`). Esc/clique fora fecha.
- Escolha do provedor em `localStorage` (`colmeia.usage.provider`), com try/catch.
- Atualiza a cada 60 s e ao abrir o popover.
- Cores: ok = acento/dourado, warn ≥70% = âmbar, exhausted = vermelho (`--danger`).
  Minimalista, mesmo estilo do menu Agentes.

## Regras
Siga `docs/WORKER.md`. Só seus arquivos; programe contra este contrato. `npx tsc --noEmit -p
tsconfig.json` limpo (o CLI: `node --check`). Sem build, sem reiniciar, sem commit. `RESUMO:` curto.
