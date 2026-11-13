import { memo, useState } from 'react'
import { BaseEdge, getBezierPath, Position, useInternalNode, useStore, type EdgeProps } from '@xyflow/react'
import { edgeMode } from '../store'
import { getSlots, rectOf, type Rect, type Side } from './slots'

const POSITION: Record<Side, Position> = {
  top: Position.Top,
  right: Position.Right,
  bottom: Position.Bottom,
  left: Position.Left
}

/** Ponto na borda do card; frac (0..1) é a posição ao longo do lado. */
function anchor(r: Rect, side: Side, frac: number) {
  switch (side) {
    case 'top':
      return { x: r.x + r.w * frac, y: r.y }
    case 'bottom':
      return { x: r.x + r.w * frac, y: r.y + r.h }
    case 'left':
      return { x: r.x, y: r.y + r.h * frac }
    case 'right':
      return { x: r.x + r.w, y: r.y + r.h * frac }
  }
}

/** Hexágono "pointy-top" de raio r centrado em (x, y). */
function hex(x: number, y: number, r: number) {
  const pts: string[] = []
  for (let i = 0; i < 6; i++) {
    const ang = (Math.PI / 3) * i - Math.PI / 2
    pts.push(`${(x + r * Math.cos(ang)).toFixed(1)},${(y + r * Math.sin(ang)).toFixed(1)}`)
  }
  return pts.join(' ')
}

/** Edge bezier: respeita lados salvos, flutua sem eles e espalha links do mesmo lado. */
function SmartEdge(props: EdgeProps) {
  const { id, source, target, style, selected } = props
  const [hover, setHover] = useState(false)
  const mode = edgeMode({ data: props.data } as never)
  // abelha só com hover/seleção ou card de ponta selecionado (evita repaint perpétuo)
  const endActive = useStore((st) => !!st.nodeLookup.get(source)?.internals.userNode.selected || !!st.nodeLookup.get(target)?.internals.userNode.selected)
  const showBee = hover || !!selected || endActive
  const sourceNode = useInternalNode(source)
  const targetNode = useInternalNode(target)
  const slot = useStore((st) => getSlots(st.edges, st.nodeLookup).get(id))
  const s = rectOf(sourceNode)
  const t = rectOf(targetNode)
  if (!s || !t || !slot) return null

  const a = anchor(s, slot.sSide, slot.sFrac)
  const b = anchor(t, slot.tSide, slot.tFrac)

  const [path, lx, ly] = getBezierPath({
    sourceX: a.x,
    sourceY: a.y,
    sourcePosition: POSITION[slot.sSide],
    targetX: b.x,
    targetY: b.y,
    targetPosition: POSITION[slot.tSide]
  })

  // seta no destino (aponta para dentro do card)
  const dir = { top: [0, 1], bottom: [0, -1], left: [1, 0], right: [-1, 0] }[slot.tSide]
  const [dx, dy] = dir
  const arrow = `${b.x},${b.y} ${b.x - dx * 9 - dy * 5},${b.y - dy * 9 + dx * 5} ${b.x - dx * 9 + dy * 5},${b.y - dy * 9 - dx * 5}`

  return (
    <g onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}>
      <BaseEdge id={id} path={path} style={mode === 'pipe' ? { ...style, strokeDasharray: '8 4' } : style} className="hive-edge" />
      <polygon className="hive-cell" points={hex(a.x, a.y, 4)} />
      <polygon className="hive-arrow" points={arrow} />
      {mode === 'pipe' && (
        <text className="hive-label" x={lx} y={ly - 6} textAnchor="middle">
          RESUMO
        </text>
      )}
      {showBee && (
        <polygon className="hive-bee" points={hex(0, 0, 2.6)}>
          <animateMotion dur="5s" repeatCount="indefinite" path={path} />
        </polygon>
      )}
    </g>
  )
}

export default memo(SmartEdge)
