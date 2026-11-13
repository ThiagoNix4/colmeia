// Cliente do daemon `colmeiad`: socket unix, JSON por linha, mux por rid, stream de eventos.
import net from 'node:net'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { spawn, execFileSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'

export type DaemonEvent = { ev: string; [k: string]: unknown }
type Pending = { resolve: (v: unknown) => void; reject: (e: Error) => void; timer: NodeJS.Timeout }

const configDir = () => path.join(process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'), 'colmeia')
export const daemonSocketPath = () =>
  process.env.COLMEIA_SOCK_PATH ||
  (process.platform === 'win32'
    ? `\\\\.\\pipe\\colmeia-${os.userInfo().username}`
    : path.join(process.env.XDG_RUNTIME_DIR || os.tmpdir(), 'colmeia.sock'))

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

export class DaemonClient {
  private sock: net.Socket | null = null
  private buf = ''
  private seq = 0
  private pending = new Map<number, Pending>()
  private handlers = new Set<(ev: DaemonEvent) => void>()
  private closeHandlers = new Set<() => void>()
  attached = false

  constructor(
    private readonly sockPath = daemonSocketPath(),
    private readonly binPath = '',
    private readonly spawnArgs: string[] = [],
    /** cli/docs esperados: se o daemon em execução aponta p/ outros, usa o mesmo prompt de reinício. */
    private readonly expectDirs?: { cli: string; docs: string },
    /** Versão do daemon difere do binário: true = reiniciar (--stop + respawn; cards morrem). */
    private readonly onVersionMismatch?: (running: string, expected: string) => Promise<boolean>
  ) {}

  /** Eventos empurrados pelo daemon (pty.data, pty.exit, canvas, note-changed, ui.req…). */
  onEvent(cb: (ev: DaemonEvent) => void) {
    this.handlers.add(cb)
    return () => this.handlers.delete(cb)
  }
  onClose(cb: () => void) {
    this.closeHandlers.add(cb)
  }

  private open(): Promise<net.Socket> {
    return new Promise((resolve, reject) => {
      const s = net.createConnection(this.sockPath)
      const fail = (e: Error) => reject(e)
      s.once('error', fail)
      s.once('connect', () => {
        s.off('error', fail)
        this.buf = ''
        s.setEncoding('utf8')
        s.on('data', (d: string) => this.onData(d))
        s.on('error', () => {})
        s.on('close', () => this.onClose_())
        resolve(s)
      })
    })
  }

  private onClose_() {
    this.sock = null
    this.attached = false
    for (const [, p] of this.pending) {
      clearTimeout(p.timer)
      p.reject(new Error('daemon desconectou'))
    }
    this.pending.clear()
    if (this.connecting) return // close provocado pelo próprio connect() (ping falho/--stop): não dispara reconexão
    for (const cb of this.closeHandlers) cb()
  }

  private onData(chunk: string) {
    this.buf += chunk
    let i: number
    while ((i = this.buf.indexOf('\n')) >= 0) {
      const line = this.buf.slice(0, i)
      this.buf = this.buf.slice(i + 1)
      if (!line.trim()) continue
      let msg: Record<string, unknown>
      try {
        msg = JSON.parse(line)
      } catch {
        continue
      }
      if (typeof msg.ev === 'string') {
        for (const h of this.handlers) h(msg as DaemonEvent)
        continue
      }
      const p = this.pending.get(msg.id as number)
      if (!p) continue
      this.pending.delete(msg.id as number)
      clearTimeout(p.timer)
      if (msg.ok) p.resolve(msg.result)
      else p.reject(new Error(String(msg.error ?? 'erro do daemon')))
    }
  }

  /** Chamada RPC; rejeita com o texto de erro do daemon. */
  call<T = unknown>(cmd: string, args: Record<string, unknown> = {}, timeoutMs = 30_000): Promise<T> {
    const s = this.sock
    if (!s || s.destroyed) return Promise.reject(new Error('daemon desconectado'))
    const id = ++this.seq
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error(`daemon: timeout em ${cmd}`))
      }, timeoutMs)
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer })
      s.write(JSON.stringify({ id, cmd, args }) + '\n')
    })
  }

  /** Fire-and-forget (write/resize/…): erros são ignorados. */
  fire(cmd: string, args: Record<string, unknown> = {}) {
    this.call(cmd, args).catch(() => {})
  }

  /** Ping: devolve versão/dirs reportados ('' / undefined se o daemon não informa) ou null se não responde. */
  private async ping(): Promise<{ version: string; cliDir?: string; docsDir?: string } | null> {
    try {
      this.sock = await this.open()
      const r = await this.call<{ version?: string; cliDir?: string; docsDir?: string } | string | null>('ping', {}, 3000)
      if (!r || typeof r !== 'object') return { version: '' }
      return {
        version: typeof r.version === 'string' ? r.version : '',
        cliDir: typeof r.cliDir === 'string' ? r.cliDir : undefined,
        docsDir: typeof r.docsDir === 'string' ? r.docsDir : undefined
      }
    } catch {
      this.sock?.destroy()
      this.sock = null
      return null
    }
  }

  /** Versão do binário em disco (`colmeiad --version`); '' se indisponível. */
  private binVersion(): string {
    try {
      return execFileSync(this.binPath, ['--version'], { timeout: 2000, encoding: 'utf8' }).trim()
    } catch {
      return ''
    }
  }

  private spawnDaemon() {
    fs.mkdirSync(configDir(), { recursive: true })
    const log = fs.openSync(path.join(configDir(), 'colmeiad.log'), 'a')
    const child = spawn(this.binPath, this.spawnArgs, { detached: true, windowsHide: true, cwd: configDir(), stdio: ['ignore', log, log] })
    child.on('error', () => {})
    child.unref()
    fs.closeSync(log)
  }

  /** Conecta; se o daemon não responde, sobe `colmeiad` destacado e tenta de novo (~5s). */
  async connect(): Promise<void> {
    // chamadas concorrentes compartilham a mesma tentativa (evita 2º spawn do colmeiad)
    if (this.connectP) return this.connectP
    this.connecting = true
    this.connectP = this.connect_().finally(() => {
      this.connecting = false
      this.connectP = null
    })
    return this.connectP
  }

  private connecting = false
  private connectP: Promise<void> | null = null

  private async connect_(): Promise<void> {
    const running = await this.ping()
    if (running !== null) {
      const expected = running.version ? this.binVersion() : ''
      const d = this.expectDirs
      const dirsDiffer = !!d && ((running.cliDir !== undefined && running.cliDir !== d.cli) || (running.docsDir !== undefined && running.docsDir !== d.docs))
      const verDiffer = !!running.version && !!expected && running.version !== expected
      // versão ou cli/docs diferentes: descreve os dois lados no mesmo diálogo
      const label = (v: string, cli?: string) => (dirsDiffer ? `${v || '?'}, cli: ${cli ?? '?'}` : v)
      if ((verDiffer || dirsDiffer) && this.onVersionMismatch && (await this.onVersionMismatch(label(running.version, running.cliDir), label(expected || running.version, d?.cli)))) {
        let oldPid = 0
        try {
          oldPid = parseInt(fs.readFileSync(path.join(configDir(), 'colmeiad.pid'), 'utf8').trim(), 10) || 0
        } catch {
          /* sem pidfile */
        }
        try {
          this.sock?.destroy()
          execFileSync(this.binPath, ['--stop'], { timeout: 5000 })
        } catch {
          /* segue: se ainda responder, o loop abaixo falha com mensagem clara */
        }
        for (let i = 0; i < 30 && process.platform !== 'win32' && fs.existsSync(this.sockPath); i++) await sleep(100)
        // espera o daemon antigo sair de fato (ele segura o flock do pidfile)
        for (let i = 0; i < 50 && oldPid > 0; i++) {
          try {
            process.kill(oldPid, 0)
          } catch {
            break
          }
          await sleep(100)
        }
      } else return
    }
    if (!this.binPath || !fs.existsSync(this.binPath)) throw new Error(`colmeiad não encontrado: ${this.binPath}`)
    this.spawnDaemon()
    for (let i = 0; i < 50; i++) {
      await sleep(100)
      if ((await this.ping()) !== null) return
    }
    throw new Error('colmeiad não respondeu')
  }

  private async readSecret(): Promise<string> {
    const f = path.join(configDir(), 'secret')
    // instalação nova: o daemon só cria o segredo no 1º uso (ui.attach) → cria aqui (O_EXCL, 0600; formato hex 32B)
    try {
      fs.mkdirSync(configDir(), { recursive: true })
      fs.writeFileSync(f, randomBytes(32).toString('hex'), { flag: 'wx', mode: 0o600 })
    } catch {
      /* já existe */
    }
    for (let i = 0; i < 20; i++) {
      try {
        const t = fs.readFileSync(f, 'utf8').trim()
        if (t) return t
      } catch {
        /* daemon ainda criando */
      }
      await sleep(100)
    }
    throw new Error('segredo do colmeia indisponível')
  }

  /** ui.attach: a partir daqui o daemon empurra eventos nesta conexão. */
  async attach(): Promise<{ socketPath: string; presetsPath: string; home: string }> {
    const secret = await this.readSecret()
    const r = await this.call<{ socketPath: string; presetsPath: string; home: string }>('ui.attach', { secret })
    this.attached = true
    return r
  }

  close() {
    this.sock?.destroy()
  }
}
