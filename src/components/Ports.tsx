import { Handle, Position } from '@xyflow/react'

const SIDES = [
  ['top', Position.Top],
  ['right', Position.Right],
  ['bottom', Position.Bottom],
  ['left', Position.Left]
] as const

/** Quatro pontos de link, um por lado do card. */
export default function Ports() {
  return (
    <>
      {SIDES.map(([side, position]) => (
        <Handle key={side} id={side} type="source" position={position} className={`port port-${side}`} />
      ))}
    </>
  )
}
