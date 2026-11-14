// Baixa o ConPTY moderno da Microsoft (pacote NuGet Microsoft.Windows.Console.ConPTY, MIT) e
// extrai conpty.dll + OpenConsole.exe (x64) em out/, ao lado do colmeiad.exe. Versão e sha256 fixos.
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

const VERSION = '1.25.260930003'
const SHA256 = '02b07b349af66d801159bdf9e440d4a1ce78bb951f37fc8609731665afdae7ee'
const URL = `https://api.nuget.org/v3-flatcontainer/microsoft.windows.console.conpty/${VERSION}/microsoft.windows.console.conpty.${VERSION}.nupkg`

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)))
const out = path.join(root, 'out')
const marker = path.join(out, 'conpty.version')
const files = [
  ['runtimes/win-x64/native/conpty.dll', 'conpty.dll'],
  ['build/native/runtimes/x64/OpenConsole.exe', 'OpenConsole.exe']
]

if (fs.existsSync(marker) && fs.readFileSync(marker, 'utf8').trim() === VERSION && files.every(([, n]) => fs.existsSync(path.join(out, n)))) {
  console.log(`conpty ${VERSION}: já presente`)
  process.exit(0)
}

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'colmeia-conpty-'))
try {
  const res = await fetch(URL)
  if (!res.ok) throw new Error(`download falhou: HTTP ${res.status}`)
  const buf = Buffer.from(await res.arrayBuffer())
  const got = createHash('sha256').update(buf).digest('hex')
  if (got !== SHA256) throw new Error(`sha256 diferente do fixado: ${got}`)
  const pkg = path.join(tmp, 'conpty.zip')
  fs.writeFileSync(pkg, buf)
  const dir = path.join(tmp, 'x')
  fs.mkdirSync(dir)
  try {
    execFileSync(path.join(process.env.SystemRoot || 'C:/Windows', 'System32', 'tar.exe'), ['-xf', pkg, '-C', dir], { stdio: 'inherit' }) // bsdtar do Windows 10+ lê zip
  } catch {
    execFileSync('powershell', ['-NoProfile', '-Command', `Expand-Archive -LiteralPath '${pkg}' -DestinationPath '${dir}' -Force`], { stdio: 'inherit' })
  }
  fs.mkdirSync(out, { recursive: true })
  for (const [src, name] of files) {
    const from = path.join(dir, ...src.split('/'))
    if (!fs.existsSync(from)) throw new Error(`faltou no pacote: ${src}`)
    fs.copyFileSync(from, path.join(out, name + '.tmp'))
    fs.renameSync(path.join(out, name + '.tmp'), path.join(out, name)) // atômico
  }
  fs.writeFileSync(marker, VERSION + '\n')
  console.log(`conpty ${VERSION}: ok`)
} finally {
  fs.rmSync(tmp, { recursive: true, force: true })
}
