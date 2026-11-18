#!/usr/bin/env node
// CLI do Colmeia: controla os cards do app via socket Unix.
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'
import { execFile } from 'node:child_process'
import fs from 'node:fs'
import { fileURLToPath } from 'node:url'

// Instância (ver electron/instance.ts): cada variante do Colmeia tem seu daemon; os cards herdam COLMEIA_INSTANCE.
const INSTANCE =
  (process.env.COLMEIA_INSTANCE || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]/g, '')
    .replace(/^\.+|\.+$/g, '') || 'colmeia'
const SOCK =
  process.env.COLMEIA_SOCK ||
  process.env.COLMEIA_SOCK_PATH ||
  (process.platform === 'win32'
    ? `\\\\.\\pipe\\${INSTANCE}-${os.userInfo().username}`
    : path.join(process.env.XDG_RUNTIME_DIR || os.tmpdir(), `${INSTANCE}.sock`))

const NOTE_HELP = `  colmeia note ls [--all]                       lista as notas ligadas a você (--all: todas, com marca [ligada])
  colmeia note new <título> [--near ID]         cria uma nota (imprime o caminho do .md)
  colmeia note read <ref>                       imprime o conteúdo (cru, para pipe)
  colmeia note path <ref>                       imprime só o caminho absoluto
  colmeia note write <ref> <texto...|->         sobrescreve o conteúdo
  colmeia note append <ref> <texto...|->        acrescenta no fim
  colmeia note rm <ref> [--file]                remove do canvas (--file apaga o .md também)`

const REF_NOTE = '<ref> aceita o id, um prefixo dele ou o título da nota.'

const HELP = `colmeia — controla o Colmeia

  colmeia list                                  lista os cards
  colmeia presets                               lista os presets disponíveis
  colmeia preset new <título> --tool claude|opencode --model <m> [--worker] | preset rm <id> | preset reset <id>
  colmeia tidy                                  arruma cards + notas numa grade (Gerente, operárias, notas)
  colmeia spawn <preset> [--title T] [--cwd D] [--near ID] [--model M]
                                               cria um card (imprime o id)
  colmeia send <id> <texto...> [--no-enter]     escreve no card e aperta Enter
  colmeia key <id> <ctrl-c|esc|enter|tab|up|down>  envia uma tecla
  colmeia broadcast <texto...> [--ids a,b]      envia para vários (padrão: todos menos você)
  colmeia read <id> [--lines N] [--raw]         últimas N linhas (padrão 40), compactas
  colmeia result <id> [--lines N]               só o trecho a partir do último RESUMO: (compacto)
  colmeia wait <id...> [--all|--any] [--idle 5] [--timeout 600]
                                               espera cards ficarem ociosos (padrão: --all)
  colmeia ask <id> <texto...> [--idle 8] [--lines N] [--raw] [--result]
                                               send + wait + read numa tacada só (--result: como result)
  colmeia link <a> <b> [--pipe]                 aresta a→b: a comanda b (card↔nota: ler/escrever). --pipe: encaminha o RESUMO de a para b
  colmeia unlink <a> <b>                        remove a conexão entre dois cards
  colmeia focus <id>                            centraliza o card na tela
  colmeia kill <id>                             fecha o card (mata a sessão)
  colmeia sessions                              lista os cards e se têm sessão persistente (daemon)
  colmeia detach <id>                           tira o card da tela mantendo a sessão viva (não é kill)
  colmeia screenshot [arquivo.png]              captura a janela do canvas
  colmeia agents                                inventário de agentes/modelos no PC + policy
  colmeia agents set <modelo> free|paid|auto     fixa o custo de um modelo (auto remove o override)
  colmeia --version | -v | version              versão do app
  colmeia policy [free|ask|all]                 mostra/define a política de modelos do workspace
  colmeia whoami                                id do card atual
${NOTE_HELP}

<id> aceita o id, um prefixo dele ou o título do card.
Dentro de um card, só age em cards a que você aponta (aresta você→card) e notas ligadas a você; sem link: erro.
${REF_NOTE}
Use "-" como texto para ler do stdin.
read/ask/result removem ruído de TUI (linhas vazias repetidas, molduras, rodapés); --raw devolve a tela crua.`

// Flags sem valor: não consomem o argumento seguinte.
const BOOL_FLAGS = new Set(['pipe', 'all', 'any', 'raw', 'result', 'json', 'file', 'enter', 'worker'])

const KEYS = { 'ctrl-c': '\x03', 'ctrl-d': '\x04', esc: '\x1b', enter: '\r', tab: '\t', up: '\x1b[A', down: '\x1b[B', left: '\x1b[D', right: '\x1b[C' }

function parse(argv) {
  const pos = []
  const flags = {}
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    if (a.startsWith('--')) {
      const k = a.slice(2)
      if (k.startsWith('no-')) flags[k.slice(3)] = false
      else if (!BOOL_FLAGS.has(k) && i + 1 < argv.length && !argv[i + 1].startsWith('--')) flags[k] = argv[++i]
      else flags[k] = true
    } else pos.push(a)
  }
  return { pos, flags }
}

function call(cmd, args = {}) {
  const from = process.env.COLMEIA_CARD_ID
  const token = process.env.COLMEIA_TOKEN
  if (from && args.from === undefined) args = { ...args, from }
  if (token && args.token === undefined) args = { ...args, token }
  return new Promise((resolve, reject) => {
    const sock = net.createConnection(SOCK)
    let buf = ''
    sock.setEncoding('utf8')
    sock.on('connect', () => sock.write(JSON.stringify({ id: 1, cmd, args }) + '\n'))
    sock.on('data', (d) => {
      buf += d
      const nl = buf.indexOf('\n')
      if (nl < 0) return
      sock.end()
      let res
      try { res = JSON.parse(buf.slice(0, nl)) } catch { return reject(new Error('resposta inválida do Colmeia')) }
      res.ok ? resolve(res.result) : reject(new Error(res.error))
    })
    sock.on('error', (e) =>
      reject(new Error(e.code === 'ENOENT' || e.code === 'ECONNREFUSED' ? `Colmeia não está aberto (socket ${SOCK})` : e.message))
    )
  })
}

async function readText(parts) {
  if (parts.length === 1 && parts[0] === '-') {
    // Concatena bytes antes de decodificar: decodificar por chunk quebra caracteres UTF-8
    // multibyte no meio do stream (mojibake). Também remove BOM (U+FEFF) no início.
    const chunks = []
    for await (const c of process.stdin) chunks.push(Buffer.isBuffer(c) ? c : Buffer.from(c))
    let s = Buffer.concat(chunks).toString('utf8')
    if (s.charCodeAt(0) === 0xfeff) s = s.slice(1)
    if (process.platform === 'win32') s = s.replace(/\r\n/g, '\n')
    return s.replace(/\n$/, '')
  }
  return parts.join(' ')
}

// Padrão de linhas da tela: compacto = 40; cru (--raw) = 60 para read, 80 para ask.
const DEFAULT_LINES = 40
const RAW_LINES = { read: 60, ask: 80 }
const RESULT_LINES = 200
const FOOTER = [/auto mode on/i, /shift\+tab to cycle/i, /esc to interrupt/i, /ctrl\+p commands/i, /\? for shortcuts/i, /^\s*tip:/i]
const LETTER_OR_DIGIT = /[\p{L}\p{N}]/u
const RESUMO_LINE = /^[\s⏺•●*>\-│]*RESUMO:/u
// O que encerra o bloco RESUMO: linha vazia, rodapé de TUI ou linha de prompt.
const SHELL_PROMPT = /^PS [A-Za-z]:\\[^>]*>\s*$|^\S+@\S+:\S*[$#%]\s*$|^[$%#>❯]\s*$/u
const RESUMO_END = [/^\s*$/, SHELL_PROMPT, /▣/u, /^\s*build\s+·/i, /^\s*[>$❯]\s*$/u, /^\s*·/u]
const RESUMO_MAX_LINES = 8

// Remove ruído de TUI: espaços à direita, molduras (linhas sem letra/dígito),
// rodapés conhecidos e linhas vazias repetidas (máx. 1 seguida).
function compact(text) {
  const out = []
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\s+$/, '')
    if (line === '') {
      if (out.length && out[out.length - 1] !== '') out.push('')
      continue
    }
    if (!LETTER_OR_DIGIT.test(line)) continue
    if (FOOTER.some((re) => re.test(line))) continue
    out.push(line)
  }
  while (out.length && out[out.length - 1] === '') out.pop()
  return out.join('\n')
}

// Imprime só o bloco do último RESUMO: — no máximo 8 linhas, parando em linha vazia,
// rodapé de TUI (Build ·, ▣) ou linha de prompt. Sem RESUMO,: últimas 15 linhas + aviso.
function printResult(text) {
  const all = compact(text).split('\n')
  let idx = -1
  all.forEach((l, i) => {
    if (RESUMO_LINE.test(l)) idx = i
  })
  if (idx < 0) {
    console.error('[sem RESUMO: na tela; mostrando as últimas 15 linhas]')
    return console.log(all.slice(-15).join('\n'))
  }
  const out = []
  for (const line of all.slice(idx)) {
    if (out.length >= RESUMO_MAX_LINES) break
    if (RESUMO_END.some((re) => re.test(line))) break
    out.push(line)
  }
  while (out.length > 1 && SHELL_PROMPT.test(out[out.length - 1])) out.pop()
  console.log(out.join('\n'))
}

// Inventário de agentes (colmeia agents)
const AGENT_BINS = ['claude', 'opencode', 'codex', 'gemini', 'qwen', 'aider', 'cursor-agent', 'ollama', 'crush', 'goose']
// lazy: os.userInfo() lança sem entrada no passwd
function cacheFile() {
  let u
  try { u = os.userInfo().username } catch { u = process.env.USER || process.env.USERNAME || 'u' }
  return path.join(os.tmpdir(), `colmeia-agents-${u}.json`)
}

// Cache compartilhado: inventário de agentes e overrides de custo vivem no mesmo arquivo.
function readCache() {
  try { return JSON.parse(fs.readFileSync(cacheFile(), 'utf8')) } catch { return {} }
}
function writeCache(c) {
  const f = cacheFile()
  try { fs.writeFileSync(f + '.tmp', JSON.stringify(c)); fs.renameSync(f + '.tmp', f) } catch {}
}

// `colmeia agents set <modelo> free|paid` persiste o override no cache.
function setCost(model, value) {
  const c = readCache()
  c._cost = c._cost || {}
  if (value === 'auto') delete c._cost[model.toLowerCase()]
  else c._cost[model.toLowerCase()] = value
  writeCache(c)
}

function costOf(tool, model) {
  const m = `${tool} ${model}`.toLowerCase()
  const ov = (readCache()._cost || {})[(model || '').toLowerCase()]
  if (ov === 'free' || ov === 'paid') return ov
  // opencode-go/*-free, qualquer *free e opencode/big-pickle são grátis.
  if (/free|big-pickle/.test(m)) return 'free'
  if (tool === 'ollama' || /\b(local|lmstudio|llama\.cpp)\b/.test(m)) return 'local'
  if (/anthropic|openai|google|claude|codex|gemini|gpt|opus|sonnet|\bo[134]\b/.test(m)) return 'paid'
  return '?'
}

// Windows: cmd.exe /d /s /c roda .cmd/.bat sem shell:true (evita DEP0190).
// O /s faz o cmd aceitar o comando entre aspas; arg com espaço é quotada.
function cmdArg(a) {
  const s = String(a)
  return /^[\w./\\:=+@-]+$/.test(s) ? s : `"${s.replace(/"/g, '\\"')}"`
}

function run(bin, args, timeout = 10000) {
  const file = whichBin(bin) || bin
  let command = file
  let cmdArgs = args
  if (process.platform === 'win32' && /\.(cmd|bat)$/i.test(file)) {
    command = process.env.ComSpec || 'cmd.exe'
    cmdArgs = ['/d', '/s', '/c', `"${[file, ...args].map(cmdArg).join(' ')}"`]
  }
  return new Promise((resolve) =>
    execFile(command, cmdArgs, { timeout, windowsHide: true, windowsVerbatimArguments: command !== file }, (e, out) => resolve(e ? '' : String(out)))
  )
}

// PATH + %APPDATA%\npm + ~/.opencode/bin (shims npm/opencode costumam morar fora do PATH do card).
function binDirs() {
  const dirs = (process.env.PATH || '').split(path.delimiter).filter(Boolean)
  const home = os.homedir()
  if (process.platform === 'win32') {
    const appdata = process.env.APPDATA || (home && path.join(home, 'AppData', 'Roaming'))
    if (appdata) dirs.push(path.join(appdata, 'npm'))
  }
  if (home) dirs.push(path.join(home, '.opencode', 'bin'))
  return dirs
}

function whichBin(bin) {
  const exts = process.platform === 'win32' ? ['.exe', '.cmd', '.bat', ''] : ['']
  for (const d of binDirs())
    for (const x of exts) if (fs.existsSync(path.join(d, bin + x))) return path.join(d, bin + x)
  return ''
}

const onPath = (bin) => !!whichBin(bin)

async function cached(key, ttlMs, fn) {
  const c = readCache()
  if (c[key] && Date.now() - c[key].t < ttlMs) return c[key].v
  const v = await fn()
  if (!v) return v // não guarda falha/vazio
  c[key] = { t: Date.now(), v }
  writeCache(c)
  return v
}

async function agents() {
  const rows = []
  const seen = new Map()
  const add = (tool, model, preset = '-') => {
    const k = `${tool}|${model || '-'}`
    const r = seen.get(k)
    if (r) { if (r[4] === '-') r[4] = preset; return }
    const exe = tool === 'shell' ? '-' : whichBin(tool) || '✗'
    const row = [tool, model || '-', costOf(tool, model || ''), exe, preset]
    seen.set(k, row)
    rows.push(row)
  }
  const presets = await call('presets').catch(() => [])
  const used = new Set()
  for (const p of presets) {
    const bin = (p.command || '').trim().split(/\s+/)[0] || 'shell'
    const model = (p.command || '').match(/(?:--model|-m)\s+(\S+)/)?.[1]
    used.add(bin)
    add(bin, model, p.id)
  }
  const present = AGENT_BINS.filter(onPath)
  for (const b of present) if (![...seen.keys()].some((k) => k.startsWith(b + '|'))) add(b, '', '-')
  if (present.includes('opencode')) {
    const out = await cached('opencode', 600000, () => run('opencode', ['models'], 10000))
    for (const l of out.split('\n').map((x) => x.trim()).filter(Boolean)) add('opencode', l)
  }
  if (present.includes('ollama')) {
    const out = await cached('ollama', 600000, () => run('ollama', ['list'], 5000))
    for (const l of out.split('\n').slice(1)) {
      const n = l.trim().split(/\s+/)[0]
      if (n) add('ollama', n)
    }
  }
  const w = [0, 1, 2, 3].map((i) => Math.max(...rows.map((r) => r[i].length)))
  for (const r of rows) console.log(`${r[0].padEnd(w[0])}  ${r[1].padEnd(w[1])}  ${r[2].padEnd(w[2])}  ${r[3].padEnd(w[3])}  ${r[4]}`)
  console.log(`policy: ${await call('policy').catch(() => 'ask (default)')}`)
}

const secs = (v, d) => Math.round((v === undefined || v === true ? d : Number(v)) * 1000)

// `idle`/`timeout` vão em segundos (ver §0); idleMs/timeoutMs mantidos para daemons antigos.
function waitArgs(ids, flags, dIdle, dTimeout) {
  return {
    ids,
    id: ids[0],
    mode: flags.any ? 'any' : 'all',
    idle: flags.idle === undefined ? dIdle : Number(flags.idle),
    timeout: flags.timeout === undefined ? dTimeout : Number(flags.timeout),
    idleMs: secs(flags.idle, dIdle),
    timeoutMs: secs(flags.timeout, dTimeout),
  }
}

// Normaliza a resposta do wait: nova ({results}) ou legada ({idle, waitedMs}).
function waitResults(r, ids) {
  if (Array.isArray(r?.results)) return r.results
  return [{
    id: r?.id ?? ids[0],
    title: r?.id ?? ids[0],
    status: r?.idle ? 'ok' : 'timeout',
    seconds: Math.round((r?.waitedMs ?? 0) / 1000),
  }]
}

async function main() {
  const [cmd, ...rest] = process.argv.slice(2)
  const { pos, flags } = parse(rest)
  const self = process.env.COLMEIA_CARD_ID

  switch (cmd) {
    case undefined:
    case 'help':
    case '-h':
    case '--help':
      return console.log(HELP)
    case '--version':
    case '-v':
    case 'version': {
      const dir = path.dirname(fileURLToPath(import.meta.url))
      for (const rel of ['../package.json', '../../package.json', '../app/package.json', '../app.asar/package.json']) {
        try {
          const v = JSON.parse(fs.readFileSync(path.join(dir, rel), 'utf8'))
          if (v.version && /colmeia/i.test(v.name || 'colmeia')) return console.log(v.version)
        } catch {}
      }
      const r = await call('ping')
      return console.log(r.appVersion ? `${r.appVersion} (${r.version})` : r.version)
    }
    case 'whoami':
      return console.log(self ?? '(fora do Colmeia)')
    case 'list': {
      const cards = await call('list')
      if (flags.json) return console.log(JSON.stringify(cards, null, 2))
      if (!cards.length) return console.log('(nenhum card)')
      for (const c of cards)
        console.log(
          `${c.id}  ${(c.title + (c.id === self ? ' (você)' : '')).padEnd(26)} ${c.preset.padEnd(18)} ${c.alive ? 'ativo' : 'saiu ' + c.exitCode}  ${c.cwd}${c.linked ? '  [ligado]' : ''}`
        )
      return
    }
    case 'agents': {
      if (pos[0] === 'set') {
        const model = pos[1]
        const value = pos[2]
        if (!model || !['free', 'paid', 'auto'].includes(value)) throw new Error('uso: colmeia agents set <modelo> free|paid|auto')
        setCost(model, value)
        return console.log(`${model}: ${value}`)
      }
      return agents()
    }
    case 'policy': {
      if (pos[0] && !['free', 'ask', 'all'].includes(pos[0])) throw new Error('uso: colmeia policy [free|ask|all]')
      return console.log(await call('policy', pos[0] ? { set: pos[0] } : {}))
    }
    case 'presets':
      // compacto: id, nome e modelo (o comando completo traz o prompt inteiro e só gasta token)
      for (const p of await call('presets')) {
        const model = (p.command || '').match(/(?:--model|-m)\s+(\S+)/)?.[1]
        const bin = (p.command || '').trim().split(/\s+/)[0] || 'shell'
        console.log(`${p.id.padEnd(20)} ${String(p.label ?? '').padEnd(14)} ${bin}${model ? ' ' + model : ''}`)
      }
      return
    case 'preset': {
      const use = 'uso: colmeia preset new <título> --tool claude|opencode --model <m> [--worker] | preset rm <id> | preset reset <id>'
      if (pos[0] === 'new' && pos[1]) {
        const r = await call('preset.new', { title: pos.slice(1).join(' '), tool: flags.tool, model: flags.model, worker: flags.worker === true })
        return console.log(r.id)
      }
      if (pos[0] === 'rm' && pos[1]) { await call('preset.rm', { id: pos.slice(1).join(' ') }); return }
      if (pos[0] === 'reset' && pos[1]) { await call('preset.reset', { id: pos.slice(1).join(' ') }); return }
      throw new Error(use)
    }
    case 'tidy': {
      const r = await call('tidy')
      return console.log(`ok: ${r.cards} cards, ${r.notes} notas`)
    }
    case 'spawn': {
      const r = await call('spawn', { preset: pos[0], title: flags.title, cwd: flags.cwd ? path.resolve(flags.cwd) : undefined, near: flags.near ?? self, model: flags.model })
      return console.log(r.id)
    }
    case 'send':
      await call('send', { id: pos[0], text: await readText(pos.slice(1)), enter: flags.enter !== false })
      return
    case 'key': {
      const data = KEYS[pos[1]?.toLowerCase()]
      if (!data) throw new Error(`tecla desconhecida. Use: ${Object.keys(KEYS).join(', ')}`)
      await call('key', { id: pos[0], data })
      return
    }
    case 'broadcast': {
      const ids = typeof flags.ids === 'string' ? flags.ids.split(',') : undefined
      const r = await call('broadcast', { text: await readText(pos), ids, exclude: self, enter: flags.enter !== false })
      return console.log(`enviado para: ${r.sent.join(', ') || '(ninguém)'}`)
    }
    case 'read': {
      const lines = flags.lines ? Number(flags.lines) : flags.raw ? RAW_LINES.read : DEFAULT_LINES
      const r = await call('read', { id: pos[0], lines })
      return console.log(flags.raw ? r.text : compact(r.text))
    }
    case 'result': {
      const r = await call('read', { id: pos[0], lines: flags.lines ? Number(flags.lines) : RESULT_LINES })
      return printResult(r.text)
    }
    case 'wait': {
      const ids = pos.slice()
      if (!ids.length) throw new Error('uso: colmeia wait <id...> [--all|--any] [--idle N] [--timeout N]')
      const results = waitResults(await call('wait', waitArgs(ids, flags, 5, 600)), ids)
      let bad = false
      for (const r of results) {
        const title = r.title || r.id
        const s = Math.round(r.seconds ?? 0)
        if (r.status === 'ok') {
          console.log(`${title}: ok após ${s}s`)
        } else if (r.status === 'error') {
          bad = true
          console.log(`${title}: ERRO após ${s}s${r.lastLine ? ' — ' + r.lastLine : ''}`)
        } else if (r.status === 'exited') {
          if (r.exitCode !== 0) bad = true
          console.log(r.exitCode === undefined || r.exitCode === null ? `${title}: saiu` : `${title}: saiu (code ${r.exitCode})`)
        } else if (r.status === 'timeout') {
          bad = true
          console.log(`${title}: timeout`)
        } else {
          console.log(`${title}: ${r.status} após ${s}s`)
        }
      }
      process.exitCode = bad ? 1 : 0
      return
    }
    case 'ask': {
      const id = pos[0]
      await call('send', { id, text: await readText(pos.slice(1)) })
      await new Promise((r) => setTimeout(r, 1500))
      const w = waitResults(await call('wait', waitArgs([id], flags, 8, 900)), [id])[0]
      if (w.status !== 'ok') console.error(`[${w.status} após ${w.seconds ?? 0}s, card pode não ter terminado]`)
      if (flags.result) {
        const r = await call('read', { id, lines: flags.lines ? Number(flags.lines) : RESULT_LINES })
        return printResult(r.text)
      }
      const lines = flags.lines ? Number(flags.lines) : flags.raw ? RAW_LINES.ask : DEFAULT_LINES
      const r = await call('read', { id, lines })
      return console.log(flags.raw ? r.text : compact(r.text))
    }
    case 'link':
      await call('link', { a: pos[0], b: pos[1], mode: flags.pipe ? 'pipe' : 'control' })
      return
    case 'unlink':
      await call('unlink', { a: pos[0], b: pos[1] })
      return
    case 'focus':
      await call('focus', { id: pos[0] })
      return
    case 'kill':
      await call('kill', { id: pos[0] })
      return
    case 'sessions': {
      const cards = await call('sessions')
      if (!cards.length) return console.log('(nenhum card)')
      for (const c of cards)
        console.log(`${c.id}  ${c.title.padEnd(26)} ${(c.alive ? 'ativo' : 'saiu').padEnd(6)} ${c.persistent ? 'daemon' : '-'}`)
      return
    }
    case 'detach':
      await call('detach', { id: pos[0] })
      return
    case 'screenshot': {
      const r = await call('screenshot', { file: path.resolve(pos[0] ?? `colmeia-${Date.now()}.png`) })
      return console.log(r.file)
    }
    case 'note': {
      const sub = pos[0]
      const ref = pos[1]
      const usage = () => console.log(`${NOTE_HELP}\n\n${REF_NOTE}`)
      switch (sub) {
        case undefined:
        case 'help':
        case '-h':
        case '--help':
          return usage()
        case 'ls': {
          const notes = await call('note.list', flags.all ? { all: true } : {})
          if (!notes.length) return console.log('(nenhuma nota)')
          for (const n of notes) console.log(`${n.id}  ${n.title.padEnd(26)} ${n.file}${flags.all && n.linked ? '  [ligada]' : ''}`)
          return
        }
        case 'new': {
          const title = pos.slice(1).join(' ')
          if (!title) throw new Error('uso: colmeia note new <título> [--near ID]')
          const note = await call('note.new', { title, near: flags.near ?? self })
          return console.log(note.file)
        }
        case 'read': {
          if (!ref) throw new Error(`uso: colmeia note read <ref>\n\n${REF_NOTE}`)
          const r = await call('note.read', { ref })
          process.stdout.write(r.text)
          if (process.stdout.isTTY && !r.text.endsWith('\n')) process.stdout.write('\n')
          return
        }
        case 'path': {
          if (!ref) throw new Error(`uso: colmeia note path <ref>\n\n${REF_NOTE}`)
          const r = await call('note.path', { ref })
          return console.log(r.file)
        }
        case 'write':
        case 'append': {
          const uso = `uso: colmeia note ${sub} <ref> <texto...|->`
          if (!ref) throw new Error(`${uso}\n\n${REF_NOTE}`)
          if (pos.length < 3) throw new Error(uso)
          const text = await readText(pos.slice(2))
          const r = await call('note.write', { ref, text, append: sub === 'append' })
          return console.log(r.file)
        }
        case 'rm': {
          if (!ref) throw new Error(`uso: colmeia note rm <ref> [--file]\n\n${REF_NOTE}`)
          await call('note.rm', { ref, file: flags.file === true ? true : undefined })
          return
        }
        default:
          throw new Error(`subcomando desconhecido: note ${sub}\n\n${NOTE_HELP}`)
      }
    }
    case 'ping':
      return console.log(await call('ping'))
    default:
      throw new Error(`comando desconhecido: ${cmd}\n\n${HELP}`)
  }
}

main().catch((e) => {
  console.error(`colmeia: ${e.message}`)
  process.exit(1)
})
