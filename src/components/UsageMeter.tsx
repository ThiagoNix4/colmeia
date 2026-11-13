import { useCallback, useEffect, useRef, useState } from 'react'
import type { UsageProvider, UsageReport, UsageWindow } from '../../electron/types'

const LS_KEY = 'colmeia.usage.provider'
const readPick = () => {
  try { return localStorage.getItem(LS_KEY) } catch { return null }
}

const fmtTime = (ms: number) => new Date(ms).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
const fmtReset = (ms?: number) => {
  if (!ms) return ''
  const diff = ms - Date.now()
  if (diff <= 0) return ''
  if (diff > 24 * 3600e3) return `reseta ${new Date(ms).toLocaleDateString('pt-BR', { weekday: 'short' })} ${fmtTime(ms)}`
  const m = Math.round(diff / 60000)
  const h = Math.floor(m / 60)
  return `reseta às ${fmtTime(ms)} · em ${h > 0 ? `${h}h ` : ''}${m % 60}m`
}

// janela mais crítica (maior pct)
const critical = (p: UsageProvider): UsageWindow | undefined =>
  p.windows.reduce<UsageWindow | undefined>((a, w) => (w.pct != null && (!a || a.pct == null || w.pct > a.pct) ? w : a), undefined)

const tone = (status: UsageProvider['status'], pct: number | null | undefined) =>
  status === 'exhausted' || (pct ?? 0) >= 100 ? 'bad' : status === 'warn' || (pct ?? 0) >= 70 ? 'warn' : status === 'unknown' ? 'off' : 'ok'

const Bar = ({ pct, t }: { pct: number | null | undefined; t: string }) => (
  <span className={`usage-bar ${t}`}><i style={{ width: `${Math.min(100, Math.max(0, pct ?? 0))}%` }} /></span>
)

export default function UsageMeter() {
  const [report, setReport] = useState<UsageReport | null>(null)
  const [open, setOpen] = useState(false)
  const [pick, setPick] = useState<string | null>(readPick)
  const [limit, setLimit] = useState('')
  const root = useRef<HTMLDivElement>(null)

  const refresh = useCallback(async () => {
    try { setReport(await window.canvas.usage.get()) } catch { /* sem dados */ }
  }, [])

  useEffect(() => {
    refresh()
    // push do main (fs.watch) — sem polling no renderer
    return window.canvas.usage.onChange(setReport)
  }, [refresh])

  useEffect(() => {
    if (!open) return
    refresh()
    const onDown = (e: MouseEvent) => { if (root.current && !root.current.contains(e.target as Node)) setOpen(false) }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, refresh])

  const providers = report?.providers ?? []
  const cur = providers.find((p) => p.id === pick) ?? providers[0]
  if (!cur) return null
  const w = critical(cur)
  const t = tone(cur.status, w?.pct)
  const text = cur.status === 'exhausted' ? 'esgotado' : w?.pct != null ? `${Math.round(w.pct)}%` : '—'

  const choose = (id: string) => {
    setPick(id)
    try { localStorage.setItem(LS_KEY, id) } catch { /* ignora */ }
  }
  const saveLimit = async () => {
    const n = parseInt(limit, 10)
    try {
      setReport(await window.canvas.usage.setSettings({ opencodeDailyLimit: n > 0 ? n : undefined }))
    } catch { /* ignora */ }
  }

  return (
    <div className="usage" ref={root}>
      <button className="ghost usage-pill" onClick={() => setOpen((o) => !o)} title="Uso do agente de IA">
        <span className={`usage-dot ${t}`} />
        <span>{cur.label}</span>
        <Bar pct={w?.pct} t={t} />
        <span className={`usage-pct ${t}`}>{text}</span>
      </button>
      {open && (
        <div className="usage-pop">
          <div className="usage-tabs">
            {providers.map((p) => (
              <button key={p.id} className={p.id === cur.id ? 'active' : ''} onClick={() => choose(p.id)}>
                {p.id === cur.id ? '✓ ' : ''}{p.label}
              </button>
            ))}
          </div>
          {cur.windows.map((x) => {
            const wt = tone(cur.status, x.pct)
            return (
              <div className="usage-win" key={x.label}>
                <div className="usage-win-head">
                  <span>{x.label}</span>
                  <span className={`usage-pct ${wt}`}>
                    {x.pct != null ? `${Math.round(x.pct)}%` : '—'}
                    {x.used != null ? ` · ${x.used}${x.limit ? `/${x.limit}` : ''}` : ''}
                  </span>
                </div>
                <Bar pct={x.pct} t={wt} />
                {fmtReset(x.resetsAt) && <div className="usage-sub">{fmtReset(x.resetsAt)}</div>}
              </div>
            )
          })}
          {cur.detail && <div className="usage-sub">{cur.detail}</div>}
          {cur.updatedAt && <div className="usage-sub">atualizado às {fmtTime(cur.updatedAt)}</div>}
          {cur.id === 'opencode' && (
            <div className="usage-limit">
              <label>Limite diário de mensagens</label>
              <input
                type="number" min={0} placeholder="sem limite" value={limit}
                onChange={(e) => setLimit(e.target.value)}
                onBlur={saveLimit}
                onKeyDown={(e) => { if (e.key === 'Enter') saveLimit() }}
              />
            </div>
          )}
        </div>
      )}
    </div>
  )
}
