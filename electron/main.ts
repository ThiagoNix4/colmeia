import { app, BrowserWindow, dialog, ipcMain, shell } from 'electron'
import path from 'node:path'
import fs from 'node:fs'
import os from 'node:os'
import { DaemonClient } from './daemonClient'
import type { LinkEdge, NoteCard, Preset, SpawnRequest, UsageSettings, Workspace } from './types'

const socketPath =
  process.env.COLMEIA_SOCK_PATH || path.join(process.env.XDG_RUNTIME_DIR || os.tmpdir(), 'colmeia.sock')

// Faz o GNOME associar a janela ao atalho colmeia.desktop (ícone na dock)
app.setName('colmeia')
app.setDesktopName('colmeia.desktop')

if (!app.requestSingleInstanceLock()) {
  app.quit()
  process.exit(0)
}

let win: BrowserWindow | null = null
// pede flush síncrono do histórico ao daemon antes de sair (atribuído em daemonMain)
let daemonFlush: () => Promise<unknown> = async () => {}

const send = (channel: string, ...args: unknown[]) => {
  if (win && !win.isDestroyed()) win.webContents.send(channel, ...args)
}

function createWindow() {
  win = new BrowserWindow({
    width: 1500,
    height: 950,
    backgroundColor: '#0f1115',
    title: 'Colmeia',
    icon: path.join(app.getAppPath(), 'assets/icon.png'),
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, '../preload/preload.cjs'),
      contextIsolation: true,
      sandbox: false
    }
  })
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })
  if (process.env.ELECTRON_RENDERER_URL) win.loadURL(process.env.ELECTRON_RENDERER_URL)
  else win.loadFile(path.join(__dirname, '../renderer/index.html'))
  win.on('closed', () => (win = null))
}


// caminho dentro do asar → unpacked (binário/CLI precisam existir no disco)
// docs/ e cli/ ficam na raiz do app (dev: raiz do projeto; empacotado: app.asar)
const resDir = (name: string) => {
  const c = [path.join(app.getAppPath(), name), path.join(process.cwd(), name)]
  return c.find((p) => fs.existsSync(p)) ?? c[0]
}
const unpacked = (p: string) => p.replace('app.asar' + path.sep, 'app.asar.unpacked' + path.sep)

/** Copia colmeiad+cli+docs p/ ~/.local/share/colmeia/<versão>/ (atômico: tmp + rename); reaproveita se já existe. */
function stageAppImage(bin: string, cli: string, docs: string) {
  const base = path.join(process.env.XDG_DATA_HOME || path.join(os.homedir(), '.local/share'), 'colmeia')
  const dest = path.join(base, app.getVersion())
  const out = { bin: path.join(dest, 'colmeiad'), cli: path.join(dest, 'cli'), docs: path.join(dest, 'docs') }
  if (fs.existsSync(out.bin) && fs.existsSync(out.cli) && fs.existsSync(out.docs)) return out
  fs.mkdirSync(base, { recursive: true })
  const tmp = `${dest}.tmp-${process.pid}`
  fs.rmSync(tmp, { recursive: true, force: true })
  fs.mkdirSync(tmp, { recursive: true })
  fs.copyFileSync(bin, path.join(tmp, 'colmeiad'))
  fs.chmodSync(path.join(tmp, 'colmeiad'), 0o755)
  fs.cpSync(cli, path.join(tmp, 'cli'), { recursive: true })
  fs.cpSync(docs, path.join(tmp, 'docs'), { recursive: true })
  fs.rmSync(dest, { recursive: true, force: true }) // cópia parcial antiga
  fs.renameSync(tmp, dest)
  return out
}

// ---- main = ponte fina; IPC do preload → socket do colmeiad ----
function daemonMain() {
  let bin = unpacked(process.env.COLMEIA_DAEMON_BIN || path.join(app.getAppPath(), 'out/colmeiad'))
  let cliDir = unpacked(resDir('cli'))
  let docsDir = unpacked(resDir('docs'))
  // AppImage: o mount some ao fechar o app, mas o daemon sobrevive → copia p/ ~/.local/share/colmeia/<versão>/
  if (process.env.APPIMAGE) {
    try {
      const r = stageAppImage(bin, cliDir, docsDir)
      bin = r.bin
      cliDir = r.cli
      docsDir = r.docs
    } catch (e) {
      dialog.showErrorBox('Colmeia', `Falha ao copiar o daemon do AppImage: ${e instanceof Error ? e.message : e}`)
      app.exit(1)
    }
  }
  // Versão diferente do binário: pergunta ao usuário (reiniciar mata os cards vivos)
  const askRestart = async (running: string, expected: string) => {
    const r = await dialog.showMessageBox({
      type: 'question',
      buttons: ['Manter daemon atual', 'Reiniciar daemon (cards morrem)'],
      defaultId: 0,
      cancelId: 0,
      title: 'Colmeia',
      message: `O daemon em execução (${running}) é diferente do instalado (${expected}).`
    })
    return r.response === 1
  }
  const dc = new DaemonClient(
    socketPath,
    bin,
    ['--docs', docsDir, '--cli', cliDir],
    { cli: cliDir, docs: docsDir },
    askRestart
  )
  let info = { socketPath, presetsPath: '', home: os.homedir() }
  const connectAttach = async () => {
    await dc.connect()
    info = await dc.attach()
  }
  let ready: Promise<void> = connectAttach()
  const fatal = (err: unknown) => {
    dialog.showErrorBox('Colmeia', `Falha ao conectar ao daemon: ${err instanceof Error ? err.message : err}`)
    app.exit(1)
  }
  ready.catch(fatal)
  const call = async <T>(cmd: string, args: Record<string, unknown> = {}) => {
    await ready
    return dc.call<T>(cmd, args)
  }
  const fire = (cmd: string, args: Record<string, unknown> = {}) => {
    void ready.then(() => dc.fire(cmd, args)).catch(() => {})
  }

  // Offsets: fim (em bytes) do que o renderer já recebeu por card. Chunks com fim ≤ offset são
  // duplicata do replay → descartados; sobreposição parcial é aparada.
  const known = new Map<string, number>()
  const reqs = new Map<string, SpawnRequest>() // último req de cada card (re-ensure após reset/reconexão)
  const ensuring = new Map<string, { offset: number; data: string; len?: number }[]>() // chunks retidos durante ensure em voo
  const pending = new Map<string, string>() // agrupa pty.data por frame (throttle por frame)
  let flushTimer: NodeJS.Timeout | null = null
  const queue = (id: string, data: string) => {
    pending.set(id, (pending.get(id) ?? '') + data)
    flushTimer ??= setTimeout(() => {
      flushTimer = null
      for (const [cid, d] of pending) send('pty:data', cid, d)
      pending.clear()
    }, 8)
  }
  const exitedReqs = new Map<string, SpawnRequest>() // último req de cards que saíram (restart os reativa)
  const tokens = new Map<string, number>() // token por ensure em voo (L6)
  const accept = (id: string, offset: number, data: string, len?: number) => {
    const have = known.get(id)
    const raw = typeof len === 'number' ? len : Buffer.byteLength(data)
    if (have === undefined || typeof offset !== 'number') {
      queue(id, data)
      if (typeof offset === 'number') known.set(id, offset + raw)
      return
    }
    if (offset > have) {
      // lacuna: bytes perdidos → replay do zero
      const req = reqs.get(id)
      if (req) {
        void ensure(req, true).catch(() => {})
        return
      }
      // sem req: não dá p/ refazer o replay; segue de onde chegou
      known.delete(id)
      queue(id, data)
      known.set(id, offset + raw)
      return
    }
    if (offset + raw <= have) return
    // aparar só quando o texto é UTF-8 válido (byteLength == len); senão entrega inteiro
    if (offset < have && Buffer.byteLength(data) === raw) data = Buffer.from(data).subarray(have - offset).toString()
    known.set(id, offset + raw)
    queue(id, data)
  }
  /** pty.ensure com retenção de chunks até o offset do replay ser conhecido. */
  const ensure = async (req: SpawnRequest, reset = false) => {
    reqs.set(req.id, req)
    const tok = (tokens.get(req.id) ?? 0) + 1
    tokens.set(req.id, tok)
    ensuring.set(req.id, [])
    try {
      const r = await call<{ replay: string; offset: number; info: unknown }>('pty.ensure', { req })
      // o que estava pendente (ainda não enviado) já está no replay
      pending.delete(req.id)
      known.set(req.id, r.offset)
      if (reset) send('pty:data', req.id, '\x1bc' + r.replay)
      return r
    } finally {
      if (tokens.get(req.id) === tok) {
        const held = ensuring.get(req.id) ?? []
        ensuring.delete(req.id)
        for (const h of held) accept(req.id, h.offset, h.data, h.len)
      }
    }
  }

  dc.onEvent((m) => {
    switch (m.ev) {
      case 'pty.data': {
        const id = m.id as string
        const held = ensuring.get(id)
        if (held) held.push({ offset: m.offset as number, data: m.data as string, len: m.len as number | undefined })
        else accept(id, m.offset as number, m.data as string, m.len as number | undefined)
        break
      }
      case 'pty.reset': {
        // daemon perdeu chunks deste card: refaz o replay do zero
        const req = reqs.get(m.id as string)
        if (req) void ensure(req, true).catch(() => {})
        break
      }
      case 'pty.exit': {
        const xid = m.id as string
        const tail = pending.get(xid)
        if (tail !== undefined) {
          send('pty:data', xid, tail)
          pending.delete(xid)
        }
        known.delete(xid)
        const lr = reqs.get(xid)
        if (lr) exitedReqs.set(xid, lr) // guarda p/ pty:restart
        reqs.delete(xid)
        tokens.delete(xid)
        send('pty:exit', m.id, m.code)
        break
      }
      case 'canvas': {
        const ev = m.event as { type?: string; id?: string } | undefined
        if (ev?.type === 'removeCard' && ev.id) {
          known.delete(ev.id)
          reqs.delete(ev.id)
          exitedReqs.delete(ev.id)
          pending.delete(ev.id)
          tokens.delete(ev.id)
        }
        send('canvas:event', m.event)
        break
      }
      case 'note-changed':
        send('canvas:event', { type: 'note-changed', id: m.id, mtime: m.mtime })
        break
      case 'usage.changed':
        send('usage:changed', m.report)
        break
      case 'ui.req':
        if (m.op === 'screenshot') {
          const rid = m.rid
          const reply = (a: Record<string, unknown>) => dc.fire('ui.reply', { rid, ...a })
          if (!win || win.isDestroyed()) return reply({ ok: false, error: 'janela fechada' })
          win.webContents
            .capturePage()
            .then((img) => {
              fs.writeFileSync(m.file as string, img.toPNG())
              reply({ ok: true, result: m.file })
            })
            .catch((e) => reply({ ok: false, error: String(e?.message ?? e) }))
        }
        break
    }
  })
  daemonFlush = () => dc.call('app.flush', {}, 3000).catch(() => {})

  // Conexão caiu (fora do quit): reconecta, re-anexa e refaz o replay de todos os cards
  let reconnecting = false
  dc.onClose(() => {
    if (quitting || reconnecting) return
    reconnecting = true
    ensuring.clear()
    ready = (async () => {
      let last: unknown
      for (let i = 0; i < 20 && !quitting; i++) {
        try {
          await connectAttach()
          return
        } catch (e) {
          last = e
          await new Promise((r) => setTimeout(r, 1000))
        }
      }
      throw last ?? new Error('reconexão cancelada')
    })()
    ready.then(
      async () => {
        reconnecting = false
        known.clear()
        for (const req of reqs.values()) await ensure(req, true).catch(() => {})
      },
      (e) => {
        reconnecting = false
        if (!quitting) fatal(e)
      }
    )
  })

  ipcMain.handle('pty:ensure', (_e, req: SpawnRequest) => ensure(req))
  ipcMain.handle('pty:restart', async (_e, id: string) => {
    known.delete(id) // sessão nova recomeça o contador de bytes
    const er = exitedReqs.get(id)
    if (er) {
      reqs.set(id, er)
      exitedReqs.delete(id)
    }
    try {
      return await call('pty.restart', { id })
    } finally {
      known.delete(id)
    }
  })
  ipcMain.on('pty:write', (_e, id: string, data: string) => fire('pty.write', { id, data }))
  ipcMain.on('pty:resize', (_e, id: string, cols: number, rows: number) => fire('pty.resize', { id, cols, rows }))
  ipcMain.on('pty:kill', (_e, id: string) => {
    known.delete(id)
    reqs.delete(id)
    exitedReqs.delete(id)
    fire('pty.kill', { id })
  })
  ipcMain.on('pty:detachAll', () => fire('pty.detachAll'))
  ipcMain.on('pty:killAll', () => {
    known.clear()
    reqs.clear()
    exitedReqs.clear()
    fire('pty.killAll')
  })
  ipcMain.on('pty:setMeta', (_e, id: string, patch: { title?: string }) => fire('pty.setMeta', { id, patch }))
  ipcMain.handle('pty:send', (_e, ids: string[], text: string) => call('pty.send', { ids, text }))
  ipcMain.handle('pty:list', () => call('pty.list'))

  ipcMain.handle('app:info', async () => {
    await ready
    return info
  })
  ipcMain.handle('app:newId', () => call('app.newId'))
  ipcMain.handle('presets:get', () => call('presets.get'))
  ipcMain.handle('presets:save', (_e, list: Preset[]) => call('presets.save', { list }))
  ipcMain.handle('dialog:pickDir', async (_e, current?: string) => {
    const opts: Electron.OpenDialogOptions = {
      title: 'Pasta do projeto',
      defaultPath: current || os.homedir(),
      properties: ['openDirectory', 'createDirectory']
    }
    const r = win ? await dialog.showOpenDialog(win, opts) : await dialog.showOpenDialog(opts)
    return r.canceled ? null : r.filePaths[0]
  })
  ipcMain.handle('ws:list', () => call('ws.list'))
  ipcMain.handle('ws:load', (_e, name?: string) => call('ws.load', { name }))
  ipcMain.on('ws:save', (_e, ws: Workspace) => fire('ws.save', { ws }))
  ipcMain.handle('ws:delete', (_e, name: string) => call('ws.delete', { name }))
  ipcMain.handle('ws:rename', (_e, from: string, to: string) => call('ws.rename', { from, to }))
  ipcMain.on('links:set', (_e, edges: LinkEdge[]) => fire('links.set', { edges }))
  ipcMain.handle('notes:list', (_e, ws: string) => call('notes.list', { ws }))
  ipcMain.handle('notes:create', (_e, ws: string, title: string) => call('notes.create', { ws, title }))
  ipcMain.handle('notes:update', (_e, ws: string, id: string, patch: Partial<Omit<NoteCard, 'id' | 'workspace'>>) =>
    call('notes.update', { ws, id, patch })
  )
  ipcMain.handle('notes:remove', (_e, ws: string, id: string, deleteFile?: boolean) =>
    call('notes.remove', { ws, id, deleteFile })
  )
  ipcMain.handle('notes:read', (_e, ws: string, ref: string) => call('notes.read', { ws, ref }))
  ipcMain.handle('notes:write', (_e, ws: string, ref: string, text: string, baseMtime: number) =>
    call('notes.write', { ws, ref, text, baseMtime })
  )
  ipcMain.handle('notes:root', () => call('notes.root'))
  ipcMain.handle('usage:get', () => call('usage.get'))
  ipcMain.handle('usage:setSettings', (_e, s: UsageSettings) => call('usage.setSettings', { s }))

  app.whenReady().then(async () => {
    try {
      await ready
    } catch {
      return
    }
    createWindow()
  })
}

let quitting = false
daemonMain()

app.on('second-instance', () => {
  if (win) {
    if (win.isMinimized()) win.restore()
    win.focus()
  }
})

app.on('window-all-closed', () => {
  // Fechar a janela NÃO mata os terminais: o daemon segue vivo.
  app.quit()
})

// Flush do workspace no quit: pede ao renderer para salvar agora (sem debounce) e espera a resposta.
// A trava de 600ms é obrigatória: se o renderer não responder, o app TEM de fechar mesmo assim.
let flushed = false
let quitTimer: NodeJS.Timeout | null = null

let flushing = false

app.on('before-quit', (e) => {
  quitting = true
  if (!flushed && win && !win.isDestroyed()) {
    e.preventDefault()
    send('app:flush')
    quitTimer ??= setTimeout(() => {
      flushed = true
      app.quit()
    }, 600)
    return
  }
  if (!flushed) {
    // janela já fechada (X): sem app:flush; flush do histórico (mesmo socket → depois do ws.save)
    e.preventDefault()
    if (!flushing) {
      flushing = true
      void daemonFlush().then(() => {
        flushed = true
        app.quit()
      })
    }
    return
  }
  if (quitTimer) {
    clearTimeout(quitTimer)
    quitTimer = null
  }
})

ipcMain.on('app:flushed', () => {
  flushed = true
  if (quitTimer) {
    clearTimeout(quitTimer)
    quitTimer = null
  }
  // daemon: o renderer já salvou o workspace (ws.save enfileirado antes); flush do histórico e sai
  void daemonFlush().then(() => app.quit())
})
