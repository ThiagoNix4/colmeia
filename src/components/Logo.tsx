// Marca da Colmeia: três células de favo, douradas (sem fundo)
export default function Logo({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="36 36 184 184" fill="none" aria-hidden="true">
      <path d="M125.0 163.8 L95.6 180.8 L66.1 163.8 L66.1 129.8 L95.6 112.8 L125.0 129.8 Z" fill="#e8b04a" fillOpacity="0.35" stroke="#e8b04a" strokeWidth="9" strokeLinejoin="round" />
      <path d="M189.9 163.8 L160.4 180.8 L131.0 163.8 L131.0 129.8 L160.4 112.8 L189.9 129.8 Z" fill="#e8b04a" fillOpacity="0.35" stroke="#e8b04a" strokeWidth="9" strokeLinejoin="round" />
      <path d="M157.4 107.6 L128.0 124.6 L98.6 107.6 L98.6 73.6 L128.0 56.6 L157.4 73.6 Z" fill="#e8b04a" stroke="#e8b04a" strokeWidth="9" strokeLinejoin="round" />
    </svg>
  )
}
