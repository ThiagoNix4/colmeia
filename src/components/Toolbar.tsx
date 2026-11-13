import { useEffect, useRef, useState } from 'react'
import { useStore } from '../store'
import Logo from './Logo'
import AgentsMenu from './AgentsMenu'
import UsageMeter from './UsageMeter'

interface Props {
  onNewCard: (presetId: string) => void
  onNewNote: () => void
  onSwitchWorkspace: (name: string) => void
  onCreateWorkspace: (name: string) => void
  onRenameWorkspace: (from: string, to: string) => Promise<void>
  onDeleteWorkspace: (name: string) => Promise<void>
}

// encurta o home p/ ~ (Windows: sem diferenciar maiúsculas; Linux: lógica antiga)
function shortenHome(cwd: string, home: string) {
  if (/^[A-Za-z]:[\\/]/.test(cwd) || cwd.startsWith('\\')) {
    const h = home.replace(/[\\/]+$/, '')
    if (h && cwd.toLowerCase().startsWith(h.toLowerCase())) {
      const rest = cwd.slice(h.length)
      if (rest === '' || /^[\\/]/.test(rest)) return '~' + rest
    }
    return cwd
  }
  return cwd.replace(/^\/home\/[^/]+/, '~')
}

const sanitize = (raw: string) => raw.trim().replace(/[^\w.-]+/g, '_')

export default function Toolbar({ onNewCard, onNewNote, onSwitchWorkspace, onCreateWorkspace, onRenameWorkspace, onDeleteWorkspace }: Props) {
  const cwd = useStore((s) => s.cwd)
  const [home, setHome] = useState('')
  useEffect(() => {
    window.canvas.app.info().then((i) => setHome(i.home)).catch(() => {})
  }, [])
  const workspace = useStore((s) => s.workspace)
  const workspaces = useStore((s) => s.workspaces)
  const count = useStore((s) => s.nodes.filter((n) => n.type === 'terminal').length)
  const [mode, setMode] = useState<'idle' | 'create' | 'rename'>('idle')
  const settled = useRef(false)

  const openMode = (next: 'create' | 'rename') => {
    settled.current = false
    setMode(next)
  }

  const cancel = () => {
    settled.current = true
    setMode('idle')
  }

  const pickDir = async () => {
    const dir = await window.canvas.app.pickDir(cwd)
    if (dir) useStore.getState().set({ cwd: dir })
  }

  const createWorkspace = (raw: string) => {
    if (settled.current) return
    settled.current = true
    setMode('idle')
    const name = sanitize(raw)
    if (name) onCreateWorkspace(name)
  }

  const renameWorkspace = async (raw: string) => {
    if (settled.current) return
    settled.current = true
    setMode('idle')
    const name = sanitize(raw)
    if (!name || name === workspace) return
    try {
      await onRenameWorkspace(workspace, name)
    } catch (err) {
      window.alert(String(err instanceof Error ? err.message : err))
    }
  }

  const deleteWorkspace = async () => {
    if (!window.confirm(`Excluir o workspace "${workspace}"? Os terminais dele serão encerrados e os cards salvos perdidos (arquivos .md das notas ficam no vault).`)) return
    try {
      await onDeleteWorkspace(workspace)
    } catch (err) {
      window.alert(String(err instanceof Error ? err.message : err))
    }
  }

  return (
    <header className="toolbar">
      <div className="brand">
        <span className="logo"><Logo size={18} /></span> Colmeia
      </div>

      <div className="group">
        <button className="preset-btn" style={{ ['--accent' as string]: '#8b93a7' }} onClick={() => onNewCard('shell')} title="Terminal (shell puro)">
          <span className="dot" /> Terminal
        </button>
        <button
          className="preset-btn note-btn"
          style={{ ['--accent' as string]: 'var(--gold)' }}
          onClick={onNewNote}
          title="Nova nota em markdown (salva no vault)"
        >
          <span className="dot" /> + Nota
        </button>
        <AgentsMenu onNewCard={onNewCard} />
      </div>

      <span className="spacer" />

      <UsageMeter />

      <button className="ghost cwd-btn" onClick={pickDir} title="Pasta usada pelos novos cards">
        📁 {shortenHome(cwd, home)}
      </button>

      <div className="group">
        {mode === 'create' ? (
          <input
            className="ws-input"
            autoFocus
            placeholder="nome do workspace"
            onBlur={(e) => createWorkspace(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') createWorkspace(e.currentTarget.value)
              if (e.key === 'Escape') cancel()
            }}
          />
        ) : mode === 'rename' ? (
          <input
            className="ws-input"
            autoFocus
            defaultValue={workspace}
            placeholder="novo nome"
            onBlur={(e) => renameWorkspace(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') renameWorkspace(e.currentTarget.value)
              if (e.key === 'Escape') cancel()
            }}
          />
        ) : (
          <select
            className="ws-select"
            value={workspace}
            onChange={(e) => {
              if (count === 0 || window.confirm('Trocar de workspace desconecta os terminais deste (sessões persistentes continuam em segundo plano). Continuar?')) onSwitchWorkspace(e.target.value)
            }}
          >
            {workspaces.map((w) => (
              <option key={w}>{w}</option>
            ))}
          </select>
        )}
        {mode === 'idle' && (
          <div className="ws-actions">
            <button className="ghost" title="Novo workspace" onClick={() => openMode('create')}>
              ＋
            </button>
            <button className="ghost" title="Renomear este workspace" onClick={() => openMode('rename')}>
              ✎
            </button>
            <button className="ghost danger" title="Excluir este workspace" onClick={deleteWorkspace} disabled={workspaces.length <= 1}>
              🗑
            </button>
          </div>
        )}
      </div>
    </header>
  )
}
