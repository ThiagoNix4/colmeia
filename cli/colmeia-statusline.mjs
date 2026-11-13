#!/usr/bin/env node
// Statusline do Claude Code para o Colmeia.
// Lê o JSON do stdin, grava os limites de uso em ~/.config/colmeia/usage/claude.json
// e imprime uma linha curta (vira a statusline do card). Nunca lança: sempre exit 0.
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

const configBase = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config')
const usageDir = path.join(configBase, 'colmeia', 'usage')
const usageFile = path.join(usageDir, 'claude.json')

function readStdin() {
  return new Promise((resolve) => {
    let data = ''
    process.stdin.setEncoding('utf8')
    process.stdin.on('data', (c) => (data += c))
    process.stdin.on('end', () => resolve(data))
    process.stdin.on('error', () => resolve(data))
  })
}

function pctText(win) {
  const p = win && typeof win.used_percentage === 'number' ? Math.round(win.used_percentage) : null
  return p === null ? null : `${p}%`
}

function writeAtomic(file, obj) {
  fs.mkdirSync(path.dirname(file), { recursive: true })
  const tmp = file + '.tmp'
  fs.writeFileSync(tmp, JSON.stringify(obj))
  fs.renameSync(tmp, file)
}

async function main() {
  let input = {}
  try {
    const raw = await readStdin()
    input = raw.trim() ? JSON.parse(raw) : {}
  } catch {
    input = {}
  }

  const rl = input.rate_limits
  const model = input.model?.display_name ?? input.model?.id ?? 'Claude'
  const parts = [String(model)]

  if (rl) {
    try {
      writeAtomic(usageFile, {
        rate_limits: rl,
        model,
        cost_usd: input.cost?.total_cost_usd ?? null,
        updatedAt: Date.now()
      })
    } catch {
      /* sem permissão de escrita: só a linha da statusline */
    }
    const five = pctText(rl.five_hour)
    const week = pctText(rl.seven_day)
    if (five) parts.push(`conta Claude: 5h ${five}`)
    if (week) parts.push(`sem ${week}`)
  }

  process.stdout.write(parts.join(' · ') + '\n')
}

main()
  .catch(() => {})
  .finally(() => process.exit(0))
