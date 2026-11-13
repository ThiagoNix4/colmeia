import type { Edge, InternalNode } from '@xyflow/react'

export type Side = 'top' | 'right' | 'bottom' | 'left'
export type Rect = { x: number; y: number; w: number; h: number }
/** Resultado por edge: lados e fração (0..1) ao longo da borda de cada ponta. */
export type Slot = { sSide: Side; tSide: Side; sFrac: number; tFrac: number }

type Lookup = Map<string, InternalNode>

export const isSide = (v: string | null | undefined): v is Side =>
  v === 'top' || v === 'right' || v === 'bottom' || v === 'left'

export function rectOf(node: InternalNode | undefined): Rect | null {
  if (!node) return null
  const w = node.measured.width ?? node.width
  const h = node.measured.height ?? node.height
  if (w == null || h == null) return null
  const { x, y } = node.internals.positionAbsolute
  return { x, y, w, h }
}

/** Par de lados mais próximo: escolhe pelo eixo dominante entre os centros. */
export function nearestSides(source: Rect, target: Rect): [Side, Side] {
  const dx = target.x + target.w / 2 - (source.x + source.w / 2)
  const dy = target.y + target.h / 2 - (source.y + source.h / 2)
  if (Math.abs(dx) >= Math.abs(dy)) return dx >= 0 ? ['right', 'left'] : ['left', 'right']
  return dy >= 0 ? ['bottom', 'top'] : ['top', 'bottom']
}

/** Espalha só nos ~60% centrais da borda. */
const SPREAD = 0.6

function compute(edges: Edge[], lookup: Lookup): Map<string, Slot> {
  type Entry = { id: string; end: 's' | 't'; key: number }
  const groups = new Map<string, Entry[]>()
  const out = new Map<string, Slot>()

  for (const e of edges) {
    const s = rectOf(lookup.get(e.source))
    const t = rectOf(lookup.get(e.target))
    if (!s || !t) continue
    const [as, at] = nearestSides(s, t)
    const sSide = isSide(e.sourceHandle) ? e.sourceHandle : as
    const tSide = isSide(e.targetHandle) ? e.targetHandle : at
    out.set(e.id, { sSide, tSide, sFrac: 0.5, tFrac: 0.5 })
    // ordena pela posição do outro card ao longo do eixo da borda
    const axis = (side: Side, r: Rect) => (side === 'top' || side === 'bottom' ? r.x + r.w / 2 : r.y + r.h / 2)
    const add = (node: string, side: Side, end: 's' | 't', other: Rect) => {
      const k = `${node}:${side}`
      const arr = groups.get(k)
      const entry = { id: e.id, end, key: axis(side, other) }
      if (arr) arr.push(entry)
      else groups.set(k, [entry])
    }
    add(e.source, sSide, 's', t)
    add(e.target, tSide, 't', s)
  }

  for (const arr of groups.values()) {
    if (arr.length < 2) continue
    arr.sort((a, b) => a.key - b.key || (a.id < b.id ? -1 : 1))
    const n = arr.length
    arr.forEach((en, i) => {
      const f = 0.5 + ((i + 1) / (n + 1) - 0.5) * SPREAD
      const slot = out.get(en.id)!
      if (en.end === 's') slot.sFrac = f
      else slot.tFrac = f
    })
  }
  return out
}

// Memo: reaproveita o cálculo enquanto edges e geometria dos cards não mudarem.
let cache: { edges: Edge[]; sig: number[]; ids: string[]; result: Map<string, Slot> } | null = null

function signature(ids: string[], lookup: Lookup): number[] {
  const sig: number[] = []
  for (const id of ids) {
    const r = rectOf(lookup.get(id))
    sig.push(r ? r.x : NaN, r ? r.y : NaN, r ? r.w : NaN, r ? r.h : NaN)
  }
  return sig
}

export function getSlots(edges: Edge[], lookup: Lookup): Map<string, Slot> {
  if (cache && cache.edges === edges) {
    const sig = signature(cache.ids, lookup)
    if (sig.length === cache.sig.length && sig.every((v, i) => Object.is(v, cache!.sig[i]))) return cache.result
  }
  const ids = [...new Set(edges.flatMap((e) => [e.source, e.target]))]
  cache = { edges, ids, sig: signature(ids, lookup), result: compute(edges, lookup) }
  return cache.result
}
