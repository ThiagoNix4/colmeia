import { create } from 'zustand'
import { addEdge, applyEdgeChanges, applyNodeChanges, type Edge, type EdgeChange, type Node, type NodeChange } from '@xyflow/react'
import type { NoteCard, Preset, Workspace, WorkspaceEdge } from '../electron/types'

export type CardData = {
  preset: string
  title: string
  cwd: string
  status: 'starting' | 'running' | 'exited'
  exitCode: number | null
}
export type CardNode = Node<CardData, 'terminal'>

export type { NoteCard }
export type NotePatch = Partial<Omit<NoteCard, 'id' | 'workspace'>>

/** Contrato de window.canvas.notes (electron/preload.ts). */
export interface NotesApi {
  list: (workspace: string) => Promise<NoteCard[]>
  create: (workspace: string, title: string) => Promise<NoteCard>
  update: (workspace: string, id: string, patch: NotePatch) => Promise<NoteCard>
  remove: (workspace: string, id: string, deleteFile?: boolean) => Promise<void>
  read: (workspace: string, ref: string) => Promise<{ body: string; mtime: number }>
  write: (workspace: string, ref: string, text: string, baseMtime: number) => Promise<{ mtime: number }>
  root: () => Promise<string>
}

/** Guard: app antigo em execução pode não ter o canal de notas no preload. */
export function notesApi(): NotesApi | undefined {
  return window.canvas.notes as NotesApi | undefined
}

export type NoteData = { title: string; file: string }
export type NoteNode = Node<NoteData, 'note'>
export type CanvasNode = CardNode | NoteNode
export type EdgeMode = NonNullable<WorkspaceEdge['mode']>
export type EdgeData = { mode?: EdgeMode }
export const edgeMode = (e: Edge): EdgeMode => ((e.data as EdgeData | undefined)?.mode ?? 'control')

export const CARD_W = 640
export const CARD_H = 400
export const NOTE_W = 420
export const NOTE_H = 320

interface State {
  ready: boolean
  workspace: string
  workspaces: string[]
  cwd: string
  presets: Preset[]
  nodes: CanvasNode[]
  edges: Edge[]
  viewport?: Workspace['viewport']
  onNodesChange: (c: NodeChange<CanvasNode>[]) => void
  onEdgesChange: (c: EdgeChange[]) => void
  connect: (source: string, target: string, sourceHandle?: string, targetHandle?: string, mode?: EdgeMode) => void
  unlink: (a: string, b: string) => void
  removeEdge: (id: string) => void
  reverseEdge: (id: string) => void
  setEdgeMode: (id: string, mode: EdgeMode) => void
  addCard: (card: { id: string; preset: string; title: string; cwd: string }, pos: { x: number; y: number }) => void
  removeCard: (id: string) => void
  patchCard: (id: string, patch: Partial<CardData>) => void
  addNote: (note: NoteCard, pos: { x: number; y: number }) => void
  removeNote: (id: string) => void
  patchNote: (id: string, patch: Partial<NoteData>) => void
  set: (s: Partial<State>) => void
}

// Posição/tamanho da nota vão para o índice do main (não para Workspace.cards)
const noteTimers = new Map<string, ReturnType<typeof setTimeout>>()

function scheduleNoteGeometry(get: () => State, node: NoteNode) {
  const id = node.id
  const pending = noteTimers.get(id)
  if (pending) clearTimeout(pending)
  noteTimers.set(
    id,
    setTimeout(() => {
      noteTimers.delete(id)
      // fechou o card ou trocou de workspace nesse meio tempo
      if (!get().nodes.some((n) => n.id === id)) return
      const w = node.width ?? node.measured?.width
      const h = node.height ?? node.measured?.height
      const patch: NotePatch = { x: node.position.x, y: node.position.y }
      if (w != null) patch.width = w
      if (h != null) patch.height = h
      // workspace lido na hora do disparo (rename no meio do debounce)
      notesApi()
        ?.update(get().workspace, id, patch)
        .catch(() => undefined)
    }, 350)
  )
}

export const useStore = create<State>((set, get) => ({
  ready: false,
  workspace: 'default',
  workspaces: [],
  cwd: '',
  presets: [],
  nodes: [],
  edges: [],
  onNodesChange: (c) => {
    const prev = get().nodes
    const nodes = applyNodeChanges(c, prev)
    set({ nodes })
    // só mudanças de posição/tamanho de notas (evita O(n²) por frame)
    const touched = new Set<string>()
    for (const ch of c) if (ch.type === 'position' || ch.type === 'dimensions') touched.add(ch.id)
    if (touched.size) {
      for (const n of nodes) if (n.type === 'note' && touched.has(n.id)) scheduleNoteGeometry(get, n)
    }
  },
  onEdgesChange: (c) => set({ edges: applyEdgeChanges(c, get().edges) }),
  unlink: (a, b) =>
    set({ edges: get().edges.filter((e) => !((e.source === a && e.target === b) || (e.source === b && e.target === a))) }),
  removeEdge: (id) => set({ edges: get().edges.filter((e) => e.id !== id) }),
  reverseEdge: (id) =>
    set({
      edges: get().edges.map((e) =>
        e.id === id
          ? {
              ...e,
              id: `e-${e.target}-${e.source}`,
              source: e.target,
              target: e.source,
              sourceHandle: e.targetHandle,
              targetHandle: e.sourceHandle
            }
          : e
      )
    }),
  setEdgeMode: (id, mode) =>
    set({ edges: get().edges.map((e) => (e.id === id ? { ...e, data: { ...(e.data as EdgeData | undefined), mode } } : e)) }),
  connect: (source, target, sourceHandle, targetHandle, mode) => {
    if (source === target) return
    const { nodes, edges } = get()
    const a = nodes.find((n) => n.id === source)
    const b = nodes.find((n) => n.id === target)
    // ambos precisam existir; nota↔nota não tem efeito (nota não chama CLI)
    if (!a || !b || (a.type === 'note' && b.type === 'note')) return
    // upsert: mesma direção só atualiza o modo; direção oposta/nova adiciona
    const same = edges.find((e) => e.source === source && e.target === target)
    if (same) {
      if (mode && mode !== edgeMode(same))
        set({ edges: edges.map((e) => (e === same ? { ...e, data: { ...(e.data as EdgeData | undefined), mode } } : e)) })
      return
    }
    set({
      edges: addEdge(
        {
          id: `e-${source}-${target}`,
          source,
          target,
          sourceHandle,
          targetHandle,
          type: 'smart',
          data: { mode: mode ?? 'control' } satisfies EdgeData
        },
        edges
      )
    })
  },
  addCard: (card, pos) => {
    if (get().nodes.some((n) => n.id === card.id)) return
    const node: CardNode = {
      id: card.id,
      type: 'terminal',
      position: pos,
      width: CARD_W,
      height: CARD_H,
      dragHandle: '.card-header',
      data: { preset: card.preset, title: card.title, cwd: card.cwd, status: 'starting', exitCode: null }
    }
    set({ nodes: [...get().nodes.map((n) => ({ ...n, selected: false })), { ...node, selected: true }] })
  },
  addNote: (note, pos) => {
    if (get().nodes.some((n) => n.id === note.id)) return
    const width = note.width || NOTE_W
    const height = note.height || NOTE_H
    const node: NoteNode = {
      id: note.id,
      type: 'note',
      position: pos,
      width,
      height,
      dragHandle: '.card-header',
      data: { title: note.title, file: note.file }
    }
    set({ nodes: [...get().nodes.map((n) => ({ ...n, selected: false })), { ...node, selected: true }] })
    // grava já a posição do freeSpot(), senão o card volta para 0,0 ao reabrir o workspace
    notesApi()
      ?.update(get().workspace, note.id, { x: pos.x, y: pos.y, width, height })
      .catch(() => undefined)
  },
  removeCard: (id) =>
    set({
      nodes: get().nodes.filter((n) => n.id !== id),
      edges: get().edges.filter((e) => e.source !== id && e.target !== id)
    }),
  removeNote: (id) => {
    const pending = noteTimers.get(id)
    if (pending) {
      clearTimeout(pending)
      noteTimers.delete(id)
    }
    get().removeCard(id)
  },
  patchCard: (id, patch) =>
    set({
      nodes: get().nodes.map((n) => (n.id === id && n.type === 'terminal' ? { ...n, data: { ...n.data, ...patch } } : n))
    }),
  patchNote: (id, patch) => {
    const node = get().nodes.find((n) => n.id === id)
    if (!node || node.type !== 'note') return
    set({
      nodes: get().nodes.map((n) => (n.id === id && n.type === 'note' ? { ...n, data: { ...n.data, ...patch } } : n))
    })
    if (patch.title) {
      notesApi()
        ?.update(get().workspace, id, { title: patch.title })
        .catch(() => undefined)
    }
  },
  set: (s) => set(s)
}))

export function toWorkspace(s: State, viewport?: Workspace['viewport']): Workspace {
  return {
    name: s.workspace,
    cwd: s.cwd,
    viewport,
    // notas ficam no índice próprio do main, não no Workspace.cards
    cards: s.nodes
      .filter((n): n is CardNode => n.type === 'terminal')
      .map((n) => ({
        id: n.id,
        preset: n.data.preset,
        title: n.data.title,
        cwd: n.data.cwd,
        x: n.position.x,
        y: n.position.y,
        width: n.width ?? n.measured?.width ?? CARD_W,
        height: n.height ?? n.measured?.height ?? CARD_H
      })),
    edges: s.edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      ...(e.sourceHandle ? { sourceHandle: e.sourceHandle } : {}),
      ...(e.targetHandle ? { targetHandle: e.targetHandle } : {}),
      mode: edgeMode(e)
    }))
  }
}
