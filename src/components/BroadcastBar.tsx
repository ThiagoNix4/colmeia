import { useMemo, useState } from 'react'
import { useStore } from '../store'

type Target = 'selected' | 'linked' | 'all'

export default function BroadcastBar() {
  const [text, setText] = useState('')
  const [target, setTarget] = useState<Target>('selected')
  const [flash, setFlash] = useState('')
  // seletor retorna string estável (sem re-render a cada frame de drag)
  const key = useStore((s) => {
    const term = new Set(s.nodes.filter((n) => n.type === 'terminal').map((n) => n.id))
    const selected = s.nodes.filter((n) => n.selected && n.type === 'terminal').map((n) => n.id)
    if (target === 'all') return [...term].join(',')
    if (target === 'selected') return selected.join(',')
    const sel = new Set(s.nodes.filter((n) => n.selected).map((n) => n.id))
    const out = new Set<string>()
    for (const e of s.edges) {
      if (sel.has(e.source) && term.has(e.target)) out.add(e.target)
      if (sel.has(e.target) && term.has(e.source)) out.add(e.source)
    }
    return [...out].join(',')
  })
  const ids = useMemo(() => (key ? key.split(',') : []), [key])

  const send = async () => {
    if (!text.trim() || !ids.length) return
    const sent = await window.canvas.pty.send(ids, text)
    setFlash(`enviado para ${sent.length} card(s)`)
    setTimeout(() => setFlash(''), 2000)
    setText('')
  }

  return (
    <footer className="broadcast">
      <select value={target} onChange={(e) => setTarget(e.target.value as Target)}>
        <option value="selected">Selecionados</option>
        <option value="linked">Conectados aos selecionados</option>
        <option value="all">Todos</option>
      </select>
      <span className="target-count">{ids.length} alvo(s)</span>
      <textarea
        rows={1}
        value={text}
        placeholder="Mensagem para os cards… (Enter envia, Shift+Enter quebra linha)"
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault()
            send()
          }
        }}
      />
      <button className="primary" disabled={!ids.length || !text.trim()} onClick={send}>
        Enviar
      </button>
      {flash && <span className="flash">{flash}</span>}
    </footer>
  )
}
