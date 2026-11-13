import { memo, useEffect, useRef, useState } from 'react'
import { NodeResizer, useReactFlow, type NodeProps } from '@xyflow/react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { useStore, type CardNode } from '../store'
import { subscribe } from '../terminalBus'
import Ports from '../components/Ports'

const THEME = {
  background: '#12151c',
  foreground: '#d8dee9',
  cursor: '#f0c674',
  cursorAccent: '#12151c',
  selectionBackground: '#3b4252aa',
  black: '#2a2f3a',
  red: '#e06c75',
  green: '#98c379',
  yellow: '#e5c07b',
  blue: '#61afef',
  magenta: '#c678dd',
  cyan: '#56b6c2',
  white: '#d8dee9',
  brightBlack: '#5c6370',
  brightRed: '#ff7b86',
  brightGreen: '#b5e890',
  brightYellow: '#ffd68a',
  brightBlue: '#82c4ff',
  brightMagenta: '#dd9cf0',
  brightCyan: '#7fd8e3',
  brightWhite: '#ffffff'
}

const USE_WEBGL = localStorage.getItem('colmeia.webgl') === '1'
const FONT = 'CanvasTerm, "FiraCode Nerd Font Mono", monospace'
// Carrega a fonte uma vez; o xterm precisa dela pronta para medir as células
const fontReady = Promise.all([
  document.fonts.load('400 13px CanvasTerm'),
  document.fonts.load('700 13px CanvasTerm')
]).catch(() => undefined)

const isWinPath = (p: string) => /^[A-Za-z]:[\\/]/.test(p) || p.startsWith('\\')
const basename = (p: string) => (isWinPath(p) ? p.split(/[\\/]/) : p.split('/')).filter(Boolean).pop() ?? (isWinPath(p) ? p : '/')

function TerminalNode({ id, data, selected, positionAbsoluteX, positionAbsoluteY, width, height }: NodeProps<CardNode>) {
  const hostRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const preset = useStore((s) => s.presets.find((p) => p.id === data.preset))
  const patchCard = useStore((s) => s.patchCard)
  const removeCard = useStore((s) => s.removeCard)
  const { setCenter } = useReactFlow()
  const [editing, setEditing] = useState(false)

  // Monta o xterm uma vez por card
  useEffect(() => {
    const host = hostRef.current!
    const term = new Terminal({
      fontFamily: FONT,
      fontSize: 13,
      lineHeight: 1.15,
      cursorBlink: true,
      scrollback: 5000,
      allowProposedApi: true,
      theme: THEME
    })
    termRef.current = term
    const fit = new FitAddon()
    term.loadAddon(fit)
    let disposed = false
    const opened = fontReady.then(() => {
      if (disposed) return
      term.open(host)
      if (USE_WEBGL) {
        try {
          const webgl = new WebglAddon()
          webgl.onContextLoss(() => webgl.dispose())
          term.loadAddon(webgl)
        } catch {
          /* sem WebGL: usa o renderer DOM */
        }
      }
    })

    // Ctrl+Shift+C / Ctrl+Shift+V
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== 'keydown' || !e.ctrlKey || !e.shiftKey) return true
      if (e.code === 'KeyC') {
        const sel = term.getSelection()
        if (sel) navigator.clipboard.writeText(sel)
        return false
      }
      if (e.code === 'KeyV') {
        navigator.clipboard.readText().then((t) => t && term.paste(t))
        return false
      }
      return true
    })

    let ready = false
    // chunks que chegam antes do replay ficam em buffer (B4)
    const pendingChunks: string[] = []
    const unsub = subscribe(
      id,
      (chunk) => {
        if (ready) term.write(chunk)
        else pendingChunks.push(chunk)
      },
      (code) => patchCard(id, { status: 'exited', exitCode: code })
    )
    const onInput = term.onData((d) => window.canvas.pty.write(id, d))

    const doFit = () => {
      if (disposed || !host.clientWidth || !host.clientHeight) return
      try {
        fit.fit()
      } catch {
        return
      }
      window.canvas.pty.resize(id, term.cols, term.rows)
    }
    let raf = 0 as unknown as number
    const ro = new ResizeObserver(() => {
      // debounce 100ms trailing: evita tempestade de SIGWINCH ao redimensionar
      clearTimeout(raf)
      raf = window.setTimeout(doFit, 100)
    })
    ro.observe(host)

    opened
      .then(() => {
        if (disposed) return
        try {
          fit.fit()
        } catch {
          /* ainda sem tamanho */
        }
        return window.canvas.pty.ensure({ id, preset: data.preset, title: data.title, cwd: data.cwd, cols: term.cols, rows: term.rows })
      })
      .then((res) => {
        if (disposed || !res) return
        const { replay, info } = res
        if (replay) term.write(replay)
        ready = true
        for (const c of pendingChunks.splice(0)) term.write(c)
        patchCard(id, {
          status: info.alive ? 'running' : 'exited',
          exitCode: info.exitCode,
          cwd: info.cwd
        })
        doFit()
      })
      .catch((e) => {
        // sem isto o card fica mudo em "starting" quando o spawn falha
        if (disposed) return
        term.write(`\r\n\x1b[31m[colmeia] falha ao iniciar: ${e instanceof Error ? e.message : String(e)}\x1b[0m\r\n`)
        patchCard(id, { status: 'exited', exitCode: null })
      })

    return () => {
      disposed = true
      clearTimeout(raf)
      ro.disconnect()
      onInput.dispose()
      unsub()
      term.dispose()
      termRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  const focusCard = () => {
    setCenter(positionAbsoluteX + (width ?? 0) / 2, positionAbsoluteY + (height ?? 0) / 2, { zoom: 1, duration: 300 })
    setTimeout(() => termRef.current?.focus(), 320)
  }

  const commitTitle = (value: string) => {
    setEditing(false)
    const title = value.trim()
    if (!title || title === data.title) return
    patchCard(id, { title })
    window.canvas.pty.setMeta(id, { title })
  }

  const close = () => {
    window.canvas.pty.kill(id)
    removeCard(id)
  }

  const restart = async () => {
    termRef.current?.reset()
    await window.canvas.pty.restart(id)
    patchCard(id, { status: 'running', exitCode: null })
  }

  const color = preset?.color ?? '#8b93a7'

  return (
    <>
    <div className={`card ${selected ? 'selected' : ''}`} style={{ ['--accent' as string]: color }}>
      <NodeResizer isVisible={selected} minWidth={360} minHeight={200} lineClassName="resize-line" handleClassName="resize-handle" />
      <div className="card-header" onDoubleClick={(e) => e.target === e.currentTarget && focusCard()}>
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
        <span className="badge">{preset?.label ?? data.preset}</span>
        <span className="cwd" title={data.cwd}>
          {basename(data.cwd)}
        </span>
        <span className={`status ${data.status}`}>
          {data.status === 'exited' ? `saiu ${data.exitCode ?? ''}` : data.status === 'starting' ? 'iniciando' : 'ativo'}
        </span>
        <span className="spacer" />
        <code className="card-id" title="ID para a CLI colmeia">
          {id}
        </code>
        <button className="icon nodrag" title="Focar (zoom 100%)" onClick={focusCard}>
          ⌖
        </button>
        <button className="icon nodrag" title="Reiniciar" onClick={restart}>
          ↻
        </button>
        <button className="icon nodrag danger" title="Fechar" onClick={close}>
          ×
        </button>
      </div>
      <div className="term-host nodrag nowheel nopan" ref={hostRef} onMouseDown={() => termRef.current?.focus()} />
    </div>
    <Ports />
    </>
  )
}

export default memo(TerminalNode)
