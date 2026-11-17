// Mouse do xterm.js correto com zoom do canvas.
//
// O canvas (React Flow) aplica `transform: scale(zoom)` nos cards. O xterm calcula a célula do
// mouse como (clientX - rect.left) / larguraDaCélula: o rect vem escalado (tela), mas a largura da
// célula não. Fora de 100% o clique, a seleção e o mouse dos TUIs (opencode) caem na célula errada,
// tanto mais longe quanto mais longe do canto do terminal.
//
// Correção: antes do xterm ver o evento (fase de captura na janela), converte clientX/clientY
// para o espaço sem escala do terminal. A escala é medida no próprio elemento (largura na tela ÷
// largura de layout), então vale para qualquer zoom e não depende do estado do canvas.

const EVENTS = ['mousedown', 'mousemove', 'mouseup', 'click', 'dblclick', 'contextmenu', 'auxclick', 'wheel'] as const

let dragScreen: HTMLElement | null = null // terminal onde começou o arraste (seleção continua fora dele)

function screenOf(target: EventTarget | null): HTMLElement | null {
  const el = target instanceof Element ? target.closest('.xterm') : null
  return el ? (el.querySelector('.xterm-screen') as HTMLElement | null) : null
}

function remap(e: MouseEvent, screen: HTMLElement) {
  if (screen.offsetWidth === 0 || screen.offsetHeight === 0) return
  const r = screen.getBoundingClientRect()
  const sx = r.width / screen.offsetWidth
  const sy = r.height / screen.offsetHeight
  if (Math.abs(sx - 1) < 0.001 && Math.abs(sy - 1) < 0.001) return
  const x = r.left + (e.clientX - r.left) / sx
  const y = r.top + (e.clientY - r.top) / sy
  // propriedade própria do objeto sobrepõe o getter do protótipo; o evento continua nativo
  Object.defineProperty(e, 'clientX', { configurable: true, value: x })
  Object.defineProperty(e, 'clientY', { configurable: true, value: y })
  Object.defineProperty(e, 'pageX', { configurable: true, value: x + window.scrollX })
  Object.defineProperty(e, 'pageY', { configurable: true, value: y + window.scrollY })
}

function onEvent(ev: Event) {
  const e = ev as MouseEvent
  const over = screenOf(e.target)
  if (e.type === 'mousedown') dragScreen = over
  // durante o arraste o xterm escuta no document: usa o terminal onde o arraste começou
  // (no mouseup o botão já foi solto: buttons = 0, mas ainda é o fim do mesmo arraste)
  const screen = e.type === 'mouseup' ? (dragScreen ?? over) : dragScreen && e.buttons !== 0 ? dragScreen : over
  if (e.type === 'mouseup') dragScreen = null
  if (screen) remap(e, screen)
}

let installed = false
export function installXtermZoomFix() {
  if (installed) return
  installed = true
  for (const t of EVENTS) window.addEventListener(t, onEvent, { capture: true, passive: true })
}
