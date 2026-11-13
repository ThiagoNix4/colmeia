import { useEffect, useRef, useState } from 'react'
import { useStore } from '../store'
import type { AgentTool, Preset } from '../../electron/types'

interface Props {
  onNewCard: (presetId: string) => void
}

const TOOLS: { id: AgentTool; label: string; bin: string; flag: string; example: string }[] = [
  { id: 'claude', label: 'Claude Code', bin: 'claude', flag: '--model', example: 'sonnet, opus, haiku' },
  { id: 'opencode', label: 'opencode', bin: 'opencode', flag: '-m', example: 'anthropic/claude-sonnet-4' },
  { id: 'codex', label: 'Codex', bin: 'codex', flag: '-m', example: 'gpt-5-codex' },
  { id: 'gemini', label: 'Gemini CLI', bin: 'gemini', flag: '-m', example: 'gemini-2.5-pro' },
  { id: 'custom', label: 'Personalizado', bin: '', flag: '', example: '' }
]
const COLORS = ['#e8b04a', '#98c379', '#61afef', '#c678dd', '#e06c75', '#56b6c2', '#d19a66', '#abb2bf']

// monta o comando a partir de ferramenta + modelo
const RULES_MARK = '{docs}/WORKER.md'
const RULES_FLAG = `--append-system-prompt "$(cat '${RULES_MARK}')"`
const buildCommand = (tool: AgentTool, model: string, rules = false) => {
  const t = TOOLS.find((x) => x.id === tool)
  if (!t || !t.bin) return ''
  const m = model.trim()
  const base = m ? `${t.bin} ${t.flag} ${m}` : t.bin
  return tool === 'claude' && rules ? `${base} ${RULES_FLAG}` : base
}

interface Draft {
  id: string | null
  label: string
  tool: AgentTool
  model: string
  command: string
  color: string
  keepShell: boolean
  rules: boolean // regras do Colmeia (só Claude Code)
  manual: boolean // usuário editou o comando à mão
  manager?: boolean // editando o Gerente (só ferramenta + modelo)
}

// comando do Gerente (espelha managerCommand em electron/store.ts)
const managerPreview = (tool: AgentTool, model: string) => {
  const O = `"$(cat '{docs}/ORCHESTRATOR.md')"`
  const m = model.trim()
  if (tool === 'opencode') return `opencode${m ? ` -m ${m}` : ''} --prompt ${O}`
  if (tool === 'codex') return `codex${m ? ` -m ${m}` : ''} ${O}`
  if (tool === 'gemini') return `gemini${m ? ` -m ${m}` : ''} -i ${O}`
  return `claude${m ? ` --model ${m}` : ''} --effort medium --append-system-prompt ${O}`
}

const emptyDraft = (): Draft => ({
  id: null, label: '', tool: 'claude', model: '', command: buildCommand('claude', '', true), color: COLORS[0], keepShell: true, rules: true, manual: false
})

export default function AgentsMenu({ onNewCard }: Props) {
  const presets = useStore((s) => s.presets)
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const [error, setError] = useState('')
  const root = useRef<HTMLDivElement>(null)
  const hasDraft = draft !== null

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) close()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (hasDraft) { setDraft(null); setError('') } else close()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, hasDraft])

  const close = () => {
    setOpen(false)
    setDraft(null)
    setConfirmId(null)
    setError('')
  }

  const builtins = presets.filter((p) => p.builtin && p.id !== 'shell')
  const mine = presets.filter((p) => !p.builtin)

  const persist = async (list: Preset[]) => {
    try {
      const full = await window.canvas.presets.save(list)
      useStore.getState().set({ presets: full })
      return true
    } catch (err) {
      const msg = String(err instanceof Error ? err.message : err)
      setError(msg.replace(/^Error invoking remote method '[^']*': (Error: )?/, ''))
      return false
    }
  }

  const edit = (p: Preset) => {
    if (p.configurable) {
      const t = p.tool && p.tool !== 'custom' ? p.tool : 'claude'
      setConfirmId(null)
      setError('')
      setDraft({
        id: p.id, label: p.label, tool: t, model: p.model ?? '', command: managerPreview(t, p.model ?? ''), color: p.color,
        keepShell: true, rules: false, manual: false, manager: true
      })
      return
    }
    const tool = p.tool ?? 'custom'
    setConfirmId(null)
    setError('')
    const rules = tool === 'claude' && p.command.includes(RULES_MARK)
    setDraft({
      id: p.id, label: p.label, tool, model: p.model ?? '', command: p.command, color: p.color,
      keepShell: p.keepShell ?? true, rules,
      manual: tool === 'custom' || p.command !== buildCommand(tool, p.model ?? '', rules)
    })
  }

  const patch = (changes: Partial<Draft>) =>
    setDraft((d) => {
      if (!d) return d
      const next = { ...d, ...changes }
      if (next.manager) next.command = managerPreview(next.tool, next.model)
      else if (!next.manual && ('tool' in changes || 'model' in changes || 'rules' in changes) && next.tool !== 'custom') {
        next.command = buildCommand(next.tool, next.model, next.rules)
      }
      return next
    })

  const save = async () => {
    if (!draft) return
    if (draft.manager) {
      const g = presets.find((p) => p.id === draft.id)
      if (!g) return
      const item: Preset = { ...g, tool: draft.tool, model: draft.model.trim() || undefined }
      const rest = presets.filter((p) => !p.builtin)
      if (await persist([item, ...rest])) {
        setDraft(null)
        setError('')
      }
      return
    }
    const label = draft.label.trim()
    if (!label) return setError('Dê um nome ao agente.')
    const id = draft.id ?? `agent-${Date.now().toString(36)}`
    const item: Preset = {
      id, label, command: draft.command.trim(), color: draft.color, keepShell: draft.keepShell,
      tool: draft.tool, model: draft.tool === 'custom' ? undefined : draft.model.trim() || undefined
    }
    const list = draft.id ? mine.map((p) => (p.id === id ? item : p)) : [...mine, item]
    if (await persist(list)) {
      setDraft(null)
      setError('')
    }
  }

  const remove = async (id: string) => {
    setConfirmId(null)
    setError('')
    await persist(mine.filter((p) => p.id !== id))
  }

  const launch = (id: string) => {
    onNewCard(id)
    close()
  }

  const row = (p: Preset, editable: boolean) => (
    <div key={p.id} className="agent-row" style={{ ['--accent' as string]: p.color }}>
      {confirmId === p.id ? (
        <div className="agent-confirm">
          <span>Excluir “{p.label}”?</span>
          <button className="danger-btn" onClick={() => remove(p.id)}>Excluir</button>
          <button className="ghost" onClick={() => setConfirmId(null)}>Cancelar</button>
        </div>
      ) : (
        <>
          <button className="agent-main" onClick={() => launch(p.id)} title={p.command || 'shell'}>
            <span className="dot" />
            <span className="agent-name">{p.label}</span>
            <span className="agent-cmd">{p.command}</span>
          </button>
          {(editable || p.configurable) && (
            <span className="agent-actions">
              <button className="icon" title="Editar" onClick={() => edit(p)}>✎</button>
              {editable && <button className="icon danger" title="Excluir" onClick={() => setConfirmId(p.id)}>🗑</button>}
            </span>
          )}
        </>
      )}
    </div>
  )

  const tool = TOOLS.find((t) => t.id === draft?.tool)

  return (
    <div className="agents" ref={root}>
      <button className="preset-btn" onClick={() => (open ? close() : setOpen(true))} title="Seus agentes">
        Agentes ▾
      </button>
      {open && (
        <div className="agents-pop">
          {draft ? (
            <form className="agent-form" onSubmit={(e) => { e.preventDefault(); void save() }}>
              {!draft.manager && (
                <label>Nome
                  <input autoFocus value={draft.label} placeholder="Claude Sonnet" onChange={(e) => patch({ label: e.target.value })} />
                </label>
              )}
              <label>Ferramenta
                <select value={draft.tool} onChange={(e) => patch({ tool: e.target.value as AgentTool, manual: e.target.value === 'custom' })}>
                  {TOOLS.filter((t) => !draft.manager || t.id !== 'custom').map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
                </select>
              </label>
              {draft.tool !== 'custom' && (
                <label>Modelo
                  <input value={draft.model} placeholder={tool?.example} onChange={(e) => patch({ model: e.target.value })} />
                </label>
              )}
              {draft.tool === 'claude' && !draft.manager && (
                <label className="check">
                  <input type="checkbox" checked={draft.rules} onChange={(e) => patch({ rules: e.target.checked })} />
                  Regras do Colmeia (economia de token)
                </label>
              )}
              <label>Comando
                <input value={draft.command} readOnly={draft.manager} placeholder="comando a executar" onChange={(e) => patch({ command: e.target.value, manual: true })} />
              </label>
              {!draft.manager && <div className="agent-colors">
                {COLORS.map((c) => (
                  <button key={c} type="button" className={`swatch${draft.color === c ? ' on' : ''}`} style={{ background: c }} onClick={() => patch({ color: c })} />
                ))}
                <input type="color" value={draft.color} onChange={(e) => patch({ color: e.target.value })} />
              </div>}
              {!draft.manager && <label className="check">
                <input type="checkbox" checked={draft.keepShell} onChange={(e) => patch({ keepShell: e.target.checked })} />
                manter shell ao sair
              </label>}
              {error && <div className="agent-error">{error}</div>}
              <div className="agent-form-actions">
                <button type="button" className="ghost" onClick={() => { setDraft(null); setError('') }}>Cancelar</button>
                <button type="submit" className="primary">Salvar</button>
              </div>
            </form>
          ) : (
            <>
              {builtins.map((p) => row(p, false))}
              {builtins.length > 0 && <div className="agents-divider" />}
              {mine.length === 0 ? <div className="agents-empty">Nenhum agente ainda — crie o seu</div> : mine.map((p) => row(p, true))}
              {error && <div className="agent-error">{error}</div>}
              <div className="agents-divider" />
              <button className="agents-new" onClick={() => { setError(''); setDraft(emptyDraft()) }}>+ Novo agente</button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
