/** Ferramenta usada pelo formulário de agente (gera o comando a partir do modelo). */
export type AgentTool = 'claude' | 'opencode' | 'codex' | 'gemini' | 'custom'

export interface Preset {
  id: string
  label: string
  /** Comando executado no shell de login. Vazio = shell interativo puro. */
  command: string
  color: string
  /** Mantém um shell aberto quando o comando termina. */
  keepShell?: boolean
  /** shell e claude-manager: não editáveis nem excluíveis. */
  builtin?: boolean
  /** Só o Gerente: builtin, mas com ferramenta/modelo escolhíveis. */
  configurable?: boolean
  /** Só para o formulário reabrir preenchido. */
  tool?: AgentTool
  /** Só para o formulário reabrir preenchido. */
  model?: string
}

export interface CardMeta {
  id: string
  preset: string
  title: string
  cwd: string
}

export interface CardInfo extends CardMeta {
  alive: boolean
  exitCode: number | null
  pid: number | null
  /** Tem sessão persistente no daemon (sobrevive a fechar o app). */
  persistent: boolean
  /** Só em `list` com `from`: ligado ao chamador */
  linked?: boolean
}

export interface SpawnRequest extends CardMeta {
  cols?: number
  rows?: number
}

export interface WorkspaceCard extends CardMeta {
  x: number
  y: number
  width: number
  height: number
}

export interface WorkspaceEdge {
  id: string
  source: string
  target: string
  /** Lado de saída no card de origem (top|right|bottom|left). Ausente = auto. */
  sourceHandle?: string
  /** Lado de entrada no card de destino (top|right|bottom|left). Ausente = auto. */
  targetHandle?: string
  /** control (padrão) = origem comanda destino; pipe = control + encaminha RESUMO */
  mode?: EdgeMode
}
export type EdgeMode = 'control' | 'pipe'
export interface LinkEdge {
  source: string
  target: string
  mode: EdgeMode
}

export interface Workspace {
  name: string
  cwd: string
  cards: WorkspaceCard[]
  edges: WorkspaceEdge[]
  viewport?: { x: number; y: number; zoom: number }
}

export interface NoteMeta {
  id: string
  title: string
  /** Caminho absoluto do .md */
  file: string
  workspace: string
}

export interface NoteCard extends NoteMeta {
  /** Só em listas com `from` (não persiste) */
  linked?: boolean
  x: number
  y: number
  width: number
  height: number
}

export interface NotesIndex {
  workspace: string
  notes: NoteCard[]
}

/** Eventos main -> renderer disparados pela API local */
export type CanvasEvent =
  | { type: 'addCard'; card: CardMeta; near?: string }
  | { type: 'removeCard'; id: string }
  | { type: 'addNote'; note: NoteCard; near?: string }
  | { type: 'removeNote'; id: string }
  | { type: 'link'; source: string; target: string; mode: EdgeMode }
  | { type: 'unlink'; source: string; target: string }
  | { type: 'note-changed'; id: string; mtime: number }
  | { type: 'focusCard'; id: string }

/** Medidor de uso por agente de IA */
export interface UsageWindow {
  label: string
  pct: number | null
  used?: number
  limit?: number
  resetsAt?: number
}
export interface UsageProvider {
  id: 'claude' | 'opencode'
  label: string
  status: 'ok' | 'warn' | 'exhausted' | 'unknown'
  windows: UsageWindow[]
  detail?: string
  updatedAt?: number
}
export interface UsageReport {
  providers: UsageProvider[]
}
export interface UsageSettings {
  opencodeDailyLimit?: number
}
