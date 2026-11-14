import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Background,
  BackgroundVariant,
  ConnectionMode,
  Controls,
  MiniMap,
  ReactFlow,
  SelectionMode,
  useReactFlow,
  type Connection,
  type Edge
} from '@xyflow/react'
import TerminalNode from './nodes/TerminalNode'
import NoteNode from './nodes/NoteNode'
import SmartEdge from './edges/SmartEdge'
import Toolbar from './components/Toolbar'
import BroadcastBar from './components/BroadcastBar'
import { CARD_H, CARD_W, edgeMode, notesApi, toWorkspace, useStore, type CanvasNode, type CardNode, type NoteCard, type NoteNode as NoteNodeModel } from './store'
import type { CanvasEvent, Workspace } from '../electron/types'

const nodeTypes = { terminal: TerminalNode, note: NoteNode }
const edgeTypes = { smart: SmartEdge }
const defaultEdgeOptions = { type: 'smart' }


/** Erros de ipcRenderer.invoke vêm como "Error invoking remote method 'x': Error: msg". */
function ipcError(err: unknown): Error {
  const raw = err instanceof Error ? err.message : String(err)
  return new Error(raw.replace(/^.*?Error invoking remote method '[^']*':\s*(Error:\s*)?/, ''))
}

function EdgeMenu({ id, x, y, onClose }: { id: string; x: number; y: number; onClose: () => void }) {
  const st = useStore.getState()
  const edge = st.edges.find((e) => e.id === id)
  if (!edge) return null
  const mode = edgeMode(edge)
  const bothTerm = ['source', 'target'].every((k) => st.nodes.find((n) => n.id === edge[k as 'source' | 'target'])?.type === 'terminal')
  const act = (fn: () => void) => () => {
    fn()
    onClose()
  }
  return (
    <div className="edge-menu" style={{ left: x, top: y }} onMouseLeave={onClose}>
      <button onClick={act(() => st.removeEdge(id))}>Excluir</button>
      <button onClick={act(() => st.reverseEdge(id))}>Inverter direção</button>
      {bothTerm && (
        <button onClick={act(() => st.setEdgeMode(id, mode === 'pipe' ? 'control' : 'pipe'))}>
          Modo: {mode === 'pipe' ? 'pipe → control' : 'control → pipe'}
        </button>
      )}
    </div>
  )
}

export default function App() {
  const nodes = useStore((s) => s.nodes)
  const edges = useStore((s) => s.edges)
  const ready = useStore((s) => s.ready)
  const presets = useStore((s) => s.presets)
  const onNodesChange = useStore((s) => s.onNodesChange)
  const onEdgesChange = useStore((s) => s.onEdgesChange)
  const connect = useStore((s) => s.connect)
  const [menu, setMenu] = useState<{ id: string; x: number; y: number } | null>(null)
  const rf = useReactFlow()
  const rfRef = useRef(rf)
  rfRef.current = rf

  /** Posição livre perto do centro da tela (ou ao lado de outro card), sem sobrepor cards. */
  const freeSpot = useCallback((nearId?: string) => {
    const { nodes } = useStore.getState()
    const GAP = 40
    const overlaps = (p: { x: number; y: number }) =>
      nodes.some((n) => {
        const w = n.width ?? n.measured?.width ?? CARD_W
        const h = n.height ?? n.measured?.height ?? CARD_H
        return p.x < n.position.x + w + GAP && p.x + CARD_W + GAP > n.position.x && p.y < n.position.y + h + GAP && p.y + CARD_H + GAP > n.position.y
      })
    const near = nearId ? nodes.find((n) => n.id === nearId) : undefined
    const c = rfRef.current.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 })
    const origin = near
      ? { x: near.position.x + (near.width ?? CARD_W) + 80, y: near.position.y }
      : { x: c.x - CARD_W / 2, y: c.y - CARD_H / 2 }
    if (!overlaps(origin)) return origin
    // procura em anéis crescentes ao redor da origem
    const stepX = CARD_W + GAP
    const stepY = CARD_H + GAP
    for (let r = 1; r < 12; r++) {
      const ring: { x: number; y: number }[] = []
      for (let dx = -r; dx <= r; dx++)
        for (let dy = -r; dy <= r; dy++)
          if (Math.max(Math.abs(dx), Math.abs(dy)) === r) ring.push({ x: origin.x + dx * stepX, y: origin.y + dy * stepY })
      // preferir direita/baixo quando há "near"
      ring.sort((a, b) => Math.hypot(a.x - origin.x, a.y - origin.y) - Math.hypot(b.x - origin.x, b.y - origin.y))
      const hit = ring.find((p) => !overlaps(p) && (!near || p.x >= near.position.x))
      if (hit) return hit
    }
    return { x: origin.x + 40, y: origin.y + 40 }
  }, [])

  const loadWorkspace = useCallback(async (name?: string) => {
    const [ws, list, presets] = await Promise.all([
      window.canvas.ws.load(name),
      window.canvas.ws.list(),
      window.canvas.presets.get()
    ])
    // As notas vêm do índice próprio do main (mesmo store que o CLI e o Obsidian usam)
    const notes = (await notesApi()?.list(ws.name).catch(() => [])) ?? []
    const nodes: CanvasNode[] = [
      ...ws.cards.map(
        (c): CardNode => ({
          id: c.id,
          type: 'terminal',
          position: { x: c.x, y: c.y },
          width: c.width,
          height: c.height,
          dragHandle: '.card-header',
          data: { preset: c.preset, title: c.title, cwd: c.cwd, status: 'starting', exitCode: null }
        })
      ),
      ...notes.map(
        (n): NoteNodeModel => ({
          id: n.id,
          type: 'note',
          position: { x: n.x, y: n.y },
          width: n.width,
          height: n.height,
          dragHandle: '.card-header',
          data: { title: n.title, file: n.file }
        })
      )
    ]
    useStore.getState().set({
      ready: true,
      workspace: ws.name,
      workspaces: list.includes(ws.name) ? list : [...list, ws.name].sort(),
      cwd: ws.cwd,
      presets,
      nodes,
      edges: ws.edges
        .filter((e) => nodes.some((n) => n.id === e.source) && nodes.some((n) => n.id === e.target))
        .map((e) => ({
          id: e.id,
          source: e.source,
          target: e.target,
          ...(e.sourceHandle ? { sourceHandle: e.sourceHandle } : {}),
          ...(e.targetHandle ? { targetHandle: e.targetHandle } : {}),
          type: 'smart',
          data: { mode: e.mode ?? 'control' }
        }))
    })
    requestAnimationFrame(() => {
      if (ws.viewport) rfRef.current.setViewport(ws.viewport)
      else if (nodes.length) rfRef.current.fitView({ padding: 0.2 })
    })
  }, [])

  // Carga inicial
  useEffect(() => {
    loadWorkspace()
  }, [loadWorkspace])

  // Autosave (debounce 1s)
  const saveTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  // bloqueia autosave durante rename/delete (save no meio gravaria sob o nome antigo)
  const saveBlocked = useRef(false)
  const scheduleSave = useCallback(() => {
    if (saveBlocked.current) return
    clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      const s = useStore.getState()
      if (s.ready && !saveBlocked.current) window.canvas.ws.save(toWorkspace(s, rfRef.current.getViewport()))
    }, 1000)
  }, [])
  useEffect(
    () =>
      useStore.subscribe((s, prev) => {
        if (s.nodes !== prev.nodes || s.edges !== prev.edges || s.cwd !== prev.cwd) scheduleSave()
      }),
    [scheduleSave]
  )

  // Sincroniza os pares de links com o main (permissões da CLI).
  useEffect(() => {
    let last = ''
    const send = (edges: Edge[]) => {
      const list = edges.map((e) => ({ source: e.source, target: e.target, mode: edgeMode(e) }))
      // dedupe: applyEdgeChanges devolve array novo até em select/measure
      const sig = list.map((e) => `${e.source}>${e.target}:${e.mode}`).sort().join('|')
      if (sig === last) return
      last = sig
      window.canvas.links.set(list)
    }
    send(useStore.getState().edges)
    return useStore.subscribe((s, prev) => {
      if (s.edges !== prev.edges) send(s.edges)
    })
  }, [])

  /** Salva o workspace agora, sem debounce (flush do quit e beforeunload). */
  const flushSave = useCallback(() => {
    clearTimeout(saveTimer.current)
    if (saveBlocked.current) return
    const s = useStore.getState()
    if (s.ready) window.canvas.ws.save(toWorkspace(s, rfRef.current.getViewport()))
  }, [])

  // O main pede flush no quit; responder sempre para não depender da trava de 600ms.
  useEffect(() => {
    const off = window.canvas.app.onFlush(() => {
      flushSave()
      window.canvas.app.flushed()
    })
    const onBeforeUnload = () => flushSave()
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => {
      off()
      window.removeEventListener('beforeunload', onBeforeUnload)
    }
  }, [flushSave])

  // Eventos vindos da API local (CLI colmeia). Enfileira até o workspace terminar de carregar,
  // senão o set({ nodes }) do loadWorkspace apagaria cards criados nesse meio tempo.
  const pendingEvents = useRef<CanvasEvent[]>([])
  const handleEvent = useCallback(
    (ev: CanvasEvent) => {
        const st = useStore.getState()
        switch (ev.type) {
          case 'addCard':
            st.addCard(ev.card, freeSpot(ev.near))
            break
          case 'addNote':
            st.addNote(ev.note, freeSpot(ev.near))
            break
          case 'removeCard':
            st.removeCard(ev.id)
            break
          case 'removeNote':
            st.removeNote(ev.id)
            break
          case 'link':
            st.connect(ev.source, ev.target, undefined, undefined, ev.mode)
            break
          case 'unlink':
            st.unlink(ev.source, ev.target)
            break
          case 'focusCard': {
            const n = st.nodes.find((x) => x.id === ev.id)
            if (n)
              rfRef.current.setCenter(n.position.x + (n.width ?? CARD_W) / 2, n.position.y + (n.height ?? CARD_H) / 2, {
                zoom: 1,
                duration: 300
              })
            break
          }
        }
    },
    [freeSpot]
  )
  useEffect(
    () =>
      window.canvas.app.onCanvasEvent((ev) => {
        if (useStore.getState().ready) handleEvent(ev)
        else pendingEvents.current.push(ev)
      }),
    [handleEvent]
  )
  useEffect(
    () =>
      useStore.subscribe((s, prev) => {
        if (s.ready && !prev.ready && pendingEvents.current.length) {
          const queued = pendingEvents.current.splice(0)
          requestAnimationFrame(() => queued.forEach(handleEvent))
        }
      }),
    [handleEvent]
  )

  const newCard = useCallback(
    async (presetId: string) => {
      const st = useStore.getState()
      const preset = st.presets.find((p) => p.id === presetId)
      const id = await window.canvas.app.newId()
      st.addCard({ id, preset: presetId, title: `${preset?.label ?? presetId} ${id.slice(0, 4)}`, cwd: st.cwd }, freeSpot())
    },
    [freeSpot]
  )

  const newNote = useCallback(async () => {
    const st = useStore.getState()
    const api = notesApi()
    if (!api) {
      window.alert('Notas indisponíveis: reinicie o Colmeia para carregar a versão nova.')
      return
    }
    try {
      const note = await api.create(st.workspace, 'Nota')
      st.addNote(note, freeSpot())
    } catch (err) {
      window.alert(ipcError(err).message)
    }
  }, [freeSpot])

  const switchWorkspace = useCallback(
    async (name: string) => {
      const s = useStore.getState()
      if (name === s.workspace) return
      clearTimeout(saveTimer.current)
      window.canvas.ws.save(toWorkspace(s, rfRef.current.getViewport()))
      // detach: sessões persistentes sobrevivem
      window.canvas.pty.detachAll()
      useStore.getState().set({ ready: false, nodes: [], edges: [] })
      try {
        await loadWorkspace(name)
      } catch (err) {
        // falhou: volta ao workspace anterior
        console.error(err)
        window.alert(`Falha ao abrir "${name}": ${ipcError(err).message}`)
        await loadWorkspace(s.workspace).catch(() => undefined)
      }
    },
    [loadWorkspace]
  )

  const refreshWorkspaces = useCallback(async () => {
    const list = await window.canvas.ws.list()
    const s = useStore.getState()
    s.set({ workspaces: list.includes(s.workspace) ? list : [...list, s.workspace].sort() })
  }, [])

  /** Cria (ou reabre) um workspace e passa a trabalhar nele. */
  const createWorkspace = useCallback(
    async (name: string) => {
      await switchWorkspace(name)
      await refreshWorkspaces()
    },
    [switchWorkspace, refreshWorkspaces]
  )

  /** Renomeia sem reiniciar os terminais: só o arquivo e o nome em memória mudam. */
  const renameWorkspace = useCallback(
    async (from: string, to: string) => {
      const s = useStore.getState()
      saveBlocked.current = true
      clearTimeout(saveTimer.current)
      try {
        window.canvas.ws.save(toWorkspace(s, rfRef.current.getViewport()))
        const name = await window.canvas.ws.rename(from, to).catch((e) => {
          throw ipcError(e)
        })
        if (from === s.workspace) {
          useStore.getState().set({ workspace: name })
          // salva já sob o novo nome (o save anterior foi com o nome antigo)
          window.canvas.ws.save(toWorkspace(useStore.getState(), rfRef.current.getViewport()))
        }
        await refreshWorkspaces()
      } finally {
        saveBlocked.current = false
      }
    },
    [refreshWorkspaces]
  )

  const deleteWorkspace = useCallback(
    async (name: string) => {
      const current = useStore.getState().workspace
      // cancela o autosave pendente, senão ele recria o arquivo que acabamos de apagar
      saveBlocked.current = true
      clearTimeout(saveTimer.current)
      try {
        await deleteInner(name, current)
      } finally {
        saveBlocked.current = false
      }
    },
    [loadWorkspace, refreshWorkspaces]
  )
  const deleteInner = useCallback(
    async (name: string, current: string) => {
      await window.canvas.ws.remove(name).catch((e) => {
        throw ipcError(e)
      })
      const list = await window.canvas.ws.list()
      if (name !== current) {
        useStore.getState().set({ workspaces: list })
        return
      }
      window.canvas.pty.killAll()
      useStore.getState().set({ ready: false, nodes: [], edges: [] })
      try {
        await loadWorkspace(list.find((w) => w !== name) ?? 'default')
      } catch (err) {
        window.alert(`Falha ao abrir workspace: ${ipcError(err).message}`)
        await loadWorkspace('default').catch(() => undefined)
      }
      await refreshWorkspaces()
    },
    [loadWorkspace, refreshWorkspaces]
  )

  const onConnect = useCallback(
    (c: Connection) => connect(c.source, c.target, c.sourceHandle ?? undefined, c.targetHandle ?? undefined),
    [connect]
  )
  const minimapColor = useMemo(() => {
    const map = new Map(presets.map((p) => [p.id, p.color]))
    return (n: CanvasNode) => (n.type === 'note' ? '#e8b04a' : map.get(n.data.preset) ?? '#8b93a7')
  }, [presets])

  return (
    <div className="app">
      <Toolbar
        onNewCard={newCard}
        onNewNote={newNote}
        onSwitchWorkspace={switchWorkspace}
        onCreateWorkspace={createWorkspace}
        onRenameWorkspace={renameWorkspace}
        onDeleteWorkspace={deleteWorkspace}
      />
      <div className="canvas">
        {ready && (
          <ReactFlow<CanvasNode>
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            defaultEdgeOptions={defaultEdgeOptions}
            connectionMode={ConnectionMode.Loose}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onMoveEnd={scheduleSave}
            minZoom={0.1}
            maxZoom={2}
            panOnScroll
            zoomOnScroll={false}
            zoomActivationKeyCode="Control"
            panOnDrag={[1, 2]}
            selectionOnDrag
            selectionMode={SelectionMode.Partial}
            selectionKeyCode={null}
            multiSelectionKeyCode={['Control', 'Shift']}
            deleteKeyCode={['Delete', 'Backspace']}
            onSelectionStart={() => {
              // tira o foco do terminal/nota: senão o Delete vai para o xterm e não para o canvas
              if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
            }}
            onBeforeDelete={async ({ nodes: sel, edges }) => {
              // cards/notas fecham pelo próprio close() (mata o pty, salva a nota), igual ao X
              if (sel.length) {
                if (window.confirm(`Fechar ${sel.length} card(s) selecionado(s)?`))
                  window.dispatchEvent(new CustomEvent('colmeia:close-nodes', { detail: sel.map((n) => n.id) }))
                return false
              }
              return edges.length ? { nodes: [], edges } : false
            }}
            onEdgeContextMenu={(e, edge) => {
              e.preventDefault()
              setMenu({ id: edge.id, x: e.clientX, y: e.clientY })
            }}
            onPaneClick={() => setMenu(null)}
            onMoveStart={() => setMenu(null)}
            isValidConnection={(c) => {
              const ns = useStore.getState().nodes
              const a = ns.find((n) => n.id === c.source)
              const b = ns.find((n) => n.id === c.target)
              return !!a && !!b && !(a.type === 'note' && b.type === 'note')
            }}
            proOptions={{ hideAttribution: true }}
            colorMode="dark"
          >
            <Background variant={BackgroundVariant.Dots} gap={28} size={1.4} color="#2b3140" />
            <MiniMap<CanvasNode> pannable zoomable nodeColor={minimapColor} maskColor="#0b0d12cc" />
            <Controls showInteractive={false} />
          </ReactFlow>
        )}
        {ready && nodes.length === 0 && (
          <div className="empty">
            <h2>Canvas vazio</h2>
            <p>Crie um card ou uma nota pela barra de cima. Comece com um <b>Gerente</b> e alguns workers.</p>
            <p className="hint">
              Arraste o fundo com o botão do meio para mover · Ctrl+roda para zoom · arraste com o botão esquerdo para selecionar
              vários
            </p>
          </div>
        )}
      </div>
      {menu && <EdgeMenu {...menu} onClose={() => setMenu(null)} />}
      <BroadcastBar />
    </div>
  )
}

export type { Workspace }
