import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import net from 'node:net'

/** Ambiente isolado: config, notas e socket em tmp. Chamar ANTES de importar electron/*. */
export function isolate() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'colmeia-t-'))
  process.env.XDG_CONFIG_HOME = path.join(dir, 'cfg')
  process.env.COLMEIA_NOTES_DIR = path.join(dir, 'notes')
  return { dir, sock: path.join(dir, 's.sock') }
}

/** Uma requisição crua ao socket da API. */
export function rpc(sock, cmd, args) {
  return new Promise((resolve, reject) => {
    const c = net.createConnection(sock)
    let buf = ''
    c.setEncoding('utf8')
    c.on('connect', () => c.write(JSON.stringify({ id: 1, cmd, args }) + '\n'))
    c.on('data', (d) => {
      buf += d
      if (buf.includes('\n')) { c.end(); resolve(JSON.parse(buf)) }
    })
    c.on('error', reject)
  })
}

/** ctx falso: dois terminais a e b, sem PTY real. */
export function fakeCtx(ws = 'ws') {
  const cards = ['a', 'b'].map((id) => ({ id, title: id, preset: 'shell', cwd: '/', alive: true, exitCode: null, pid: 1 }))
  return {
    ptys: {
      list: () => cards,
      has: (id) => cards.some((c) => c.id === id),
      get: (id) => { const c = cards.find((x) => x.id === id); return c && { meta: c, alive: true } },
      send: () => true,
      read: async () => ''
    },
    getPresets: () => [], getDefaultCwd: () => '/', getWorkspace: () => ws,
    emit: () => {}, screenshot: async () => ''
  }
}
