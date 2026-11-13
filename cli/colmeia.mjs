#!/usr/bin/env node
// CLI do Colmeia: controla os cards do app via socket Unix.
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'

const SOCK =
  process.env.COLMEIA_SOCK ||
  process.env.COLMEIA_SOCK_PATH ||
  path.join(process.env.XDG_RUNTIME_DIR || os.tmpdir(), 'colmeia.sock')

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
  colmeia spawn <preset> [--title T] [--cwd D] [--near ID]
                                               cria um card (imprime o id)
  colmeia send <id> <texto...> [--no-enter]     escreve no card e aperta Enter
  colmeia key <id> <ctrl-c|esc|enter|tab|up|down>  envia uma tecla
  colmeia broadcast <texto...> [--ids a,b]      envia para vários (padrão: todos menos você)
  colmeia read <id> [--lines N] [--raw]         últimas N linhas (padrão 40), compactas
  colmeia result <id> [--lines N]               só o trecho a partir do último RESUMO: (compacto)
  colmeia wait <id> [--idle 5] [--timeout 600]  espera o card ficar ocioso (segundos)
  colmeia ask <id> <texto...> [--idle 8] [--lines N] [--raw] [--result]
                                               send + wait + read numa tacada só (--result: como result)
  colmeia link <a> <b> [--pipe]                 aresta a→b: a comanda b (card↔nota: ler/escrever). --pipe: encaminha o RESUMO de a para b
  colmeia unlink <a> <b>                        remove a conexão entre dois cards
  colmeia focus <id>                            centraliza o card na tela
  colmeia kill <id>                             fecha o card (mata a sessão)
  colmeia sessions                              lista os cards e se têm sessão persistente (daemon)
  colmeia detach <id>                           tira o card da tela mantendo a sessão viva (não é kill)
  colmeia screenshot [arquivo.png]              captura a janela do canvas
  colmeia whoami                                id do card atual
${NOTE_HELP}

<id> aceita o id, um prefixo dele ou o título do card.
Dentro de um card, só age em cards a que você aponta (aresta você→card) e notas ligadas a você; sem link: erro.
${REF_NOTE}
Use "-" como texto para ler do stdin.
read/ask/result removem ruído de TUI (linhas vazias repetidas, molduras, rodapés); --raw devolve a tela crua.`

// Flags sem valor: não consomem o argumento seguinte.
const BOOL_FLAGS = new Set(['pipe', 'all', 'raw', 'result', 'json', 'file', 'enter'])

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
    let s = ''
    for await (const c of process.stdin) s += c
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

// Imprime só do último RESUMO: em diante; sem RESUMO:, as últimas 15 linhas e um aviso.
function printResult(text) {
  const all = compact(text).split('\n')
  let idx = -1
  all.forEach((l, i) => {
    if (RESUMO_LINE.test(l)) idx = i
  })
  if (idx >= 0) return console.log(all.slice(idx).join('\n'))
  console.error('[sem RESUMO: na tela; mostrando as últimas 15 linhas]')
  console.log(all.slice(-15).join('\n'))
}

const secs = (v, d) => Math.round((v === undefined || v === true ? d : Number(v)) * 1000)

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
    case 'presets':
      // compacto: id, nome e modelo (o comando completo traz o prompt inteiro e só gasta token)
      for (const p of await call('presets')) {
        const model = (p.command || '').match(/(?:--model|-m)\s+(\S+)/)?.[1]
        const bin = (p.command || '').trim().split(/\s+/)[0] || 'shell'
        console.log(`${p.id.padEnd(20)} ${String(p.label ?? '').padEnd(14)} ${bin}${model ? ' ' + model : ''}`)
      }
      return
    case 'spawn': {
      const r = await call('spawn', { preset: pos[0], title: flags.title, cwd: flags.cwd ? path.resolve(flags.cwd) : undefined, near: flags.near ?? self })
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
      const r = await call('wait', { id: pos[0], idleMs: secs(flags.idle, 5), timeoutMs: secs(flags.timeout, 600) })
      console.log(r.idle ? `ocioso após ${Math.round(r.waitedMs / 1000)}s` : `timeout após ${Math.round(r.waitedMs / 1000)}s (ainda trabalhando)`)
      process.exitCode = r.idle ? 0 : 2
      return
    }
    case 'ask': {
      const id = pos[0]
      await call('send', { id, text: await readText(pos.slice(1)) })
      await new Promise((r) => setTimeout(r, 1500))
      const w = await call('wait', { id, idleMs: secs(flags.idle, 8), timeoutMs: secs(flags.timeout, 900) })
      if (!w.idle) console.error(`[timeout após ${Math.round(w.waitedMs / 1000)}s, card ainda trabalhando]`)
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
