import { memo, useEffect, useRef, useState } from 'react'
import { NodeResizer, type NodeProps } from '@xyflow/react'
import { notesApi, useStore, type NoteNode as NoteNodeModel } from '../store'
import Ports from '../components/Ports'

const AUTOSAVE_MS = 600

function NoteNode({ id, data, selected }: NodeProps<NoteNodeModel>) {
  const workspace = useStore((s) => s.workspace)
  const patchNote = useStore((s) => s.patchNote)
  const removeNote = useStore((s) => s.removeNote)
  const [text, setText] = useState('')
  const [editing, setEditing] = useState(false)
  const [save, setSave] = useState<'idle' | 'dirty' | 'saving' | 'saved' | 'conflict'>('idle')
  const [copied, setCopied] = useState(false)
  // dirty evita gravar o texto vazio do primeiro render por cima do .md
  const dirty = useRef(false)
  const textRef = useRef('')
  const mtimeRef = useRef(0)
  const seq = useRef(0)
  const conflictRef = useRef(false)
  const titleRef = useRef(data.title)
  titleRef.current = data.title
  const wsRef = useRef(workspace)
  wsRef.current = workspace

  // O .md no disco é a fonte de verdade (o Obsidian edita o mesmo arquivo)
  useEffect(() => {
    let alive = true
    notesApi()
      ?.read(workspace, id)
      .then((r) => {
        if (!alive) return
        setText(r.body)
        textRef.current = r.body
        mtimeRef.current = r.mtime
      })
      .catch(() => undefined)
    return () => {
      alive = false
    }
  }, [id, workspace])

  // Autosave com debounce: nada de escrever a cada tecla
  useEffect(() => {
    if (!dirty.current || conflictRef.current) return
    const timer = setTimeout(() => {
      const mine = ++seq.current
      const sent = textRef.current
      setSave('saving')
      notesApi()
        ?.write(wsRef.current, id, sent, mtimeRef.current)
        .then((r) => {
          mtimeRef.current = r.mtime
          // só marca limpo se nada foi digitado durante a gravação
          if (mine === seq.current && textRef.current === sent) {
            dirty.current = false
            setSave('saved')
          } else setSave('dirty')
        })
        .catch((err) => {
          if (/conflict/.test(String(err?.message ?? err))) setSave('conflict')
          else setSave('dirty')
        })
    }, AUTOSAVE_MS)
    return () => clearTimeout(timer)
  }, [text, id])

  // Fechar o card (ou trocar de workspace) antes do debounce não pode perder o texto:
  // se a gravação falhar (conflito/erro), o texto vai para uma nota nova "<título> (conflito)"
  useEffect(
    () => () => {
      if (!dirty.current) return
      dirty.current = false
      const api = notesApi()
      const ws = wsRef.current
      const text = textRef.current
      const title = titleRef.current
      const base = conflictRef.current ? -1 : mtimeRef.current
      const fork = async () => {
        const n = await api?.create(ws, `${title} (conflito)`)
        if (!n) return
        const cur = await api?.read(ws, n.id)
        await api?.write(ws, n.id, text, cur?.mtime ?? 0)
      }
      if (base < 0) fork().catch(() => undefined)
      else api?.write(ws, id, text, base).catch(() => fork().catch(() => undefined))
    },
    [id]
  )

  // Agente/Obsidian editou o .md: recarrega se limpo; se sujo, sinaliza conflito
  useEffect(
    () =>
      window.canvas.app.onCanvasEvent((ev) => {
        if (ev.type !== 'note-changed' || ev.id !== id) return
        if (ev.mtime <= mtimeRef.current) return
        if (dirty.current) {
          conflictRef.current = true
          setSave('conflict')
          return
        }
        notesApi()
          ?.read(wsRef.current, id)
          .then((r) => {
            if (dirty.current) return
            setText(r.body)
            textRef.current = r.body
            mtimeRef.current = r.mtime
            setSave('idle')
          })
          .catch(() => undefined)
      }),
    [id]
  )

  const change = (value: string) => {
    setText(value)
    textRef.current = value
    dirty.current = true
    setSave(conflictRef.current ? 'conflict' : 'dirty')
  }

  const commitTitle = (value: string) => {
    setEditing(false)
    const title = value.trim()
    if (!title || title === data.title) return
    patchNote(id, { title })
  }

  const reload = async () => {
    if (dirty.current && !window.confirm('Descartar as alterações não salvas e recarregar do disco?')) return
    const api = notesApi()
    if (!api) return
    const r = await api.read(workspace, id).catch(() => undefined)
    if (!r) return
    setText(r.body)
    textRef.current = r.body
    mtimeRef.current = r.mtime
    dirty.current = false
    conflictRef.current = false
    setSave('idle')
  }

  /** Conflito: grava o texto local por cima do disco usando o mtime atual como base. */
  const overwrite = async () => {
    const api = notesApi()
    if (!api) return
    const cur = await api.read(workspace, id).catch(() => undefined)
    if (!cur) return
    const r = await api.write(workspace, id, textRef.current, cur.mtime).catch(() => undefined)
    if (!r) return
    mtimeRef.current = r.mtime
    conflictRef.current = false
    dirty.current = false
    setSave('saved')
  }

  const copyPath = async () => {
    try {
      await navigator.clipboard.writeText(data.file)
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    } catch {
      /* sem clipboard */
    }
  }

  const close = async () => {
    const api = notesApi()
    if (dirty.current) {
      // não perde texto: em conflito/erro mantém o card aberto com a UI de conflito
      const r = conflictRef.current ? undefined : await api?.write(workspace, id, textRef.current, mtimeRef.current).catch(() => undefined)
      if (!r) {
        conflictRef.current = true
        setSave('conflict')
        return
      }
      mtimeRef.current = r.mtime
      dirty.current = false
    }
    // sai do canvas; o .md continua no vault (deleteFile = false)
    await api?.remove(workspace, id).catch(() => undefined)
    removeNote(id)
  }

  return (
    <>
    <div className={`card note-card ${selected ? 'selected' : ''}`} style={{ ['--accent' as string]: 'var(--gold)' }}>
      <NodeResizer isVisible={selected} minWidth={240} minHeight={160} lineClassName="resize-line" handleClassName="resize-handle" />
      <div className="card-header">
        <span className="dot" />
        {editing ? (
          <input
            className="title-input nodrag"
            autoFocus
            defaultValue={data.title}
            onBlur={(e) => commitTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') commitTitle(e.currentTarget.value)
              if (e.key === 'Escape') setEditing(false)
            }}
          />
        ) : (
          <span className="title" title="Duplo clique para renomear" onDoubleClick={() => setEditing(true)}>
            {data.title}
          </span>
        )}
        <span className="badge">md</span>
        {save === 'conflict' ? (
          <span className="note-status conflict nodrag">
            conflito —{' '}
            <a onClick={reload}>recarregar</a> / <a onClick={overwrite}>sobrescrever</a>
          </span>
        ) : (
          save !== 'idle' && (
            <span className={`note-status ${save}`}>{save === 'saved' ? 'salvo' : save === 'saving' ? 'salvando…' : 'editando…'}</span>
          )
        )}
        <span className="spacer" />
        <code className="card-id" title={data.file}>
          {(/^[A-Za-z]:[\\/]/.test(data.file) || data.file.startsWith('\\') ? data.file.split(/[\\/]/) : data.file.split('/')).pop()}
        </code>
        <button className="icon nodrag" title={copied ? 'Caminho copiado' : 'Copiar caminho do .md'} onClick={copyPath}>
          {copied ? '✓' : '⧉'}
        </button>
        <button className="icon nodrag" title="Recarregar do disco" onClick={reload}>
          ↻
        </button>
        <button className="icon nodrag danger" title="Fechar (mantém o arquivo)" onClick={close}>
          ×
        </button>
      </div>
      <textarea
        className="note-body nodrag nowheel nopan"
        value={text}
        onChange={(e) => change(e.target.value)}
        placeholder={'# Nota\n\nEscreva em markdown. O agente lê este arquivo direto do vault.'}
        spellCheck={false}
      />
    </div>
    <Ports />
    </>
  )
}

export default memo(NoteNode)
