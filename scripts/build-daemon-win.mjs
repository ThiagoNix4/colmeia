// Compila out/colmeiad.exe (windows/amd64) em qualquer SO, sem depender de bash.
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)))
execFileSync('go', ['build', '-trimpath', '-o', path.join(root, 'out', 'colmeiad.exe'), './cmd/colmeiad'], {
  cwd: path.join(root, 'daemon'),
  stdio: 'inherit',
  env: { ...process.env, GOOS: 'windows', GOARCH: 'amd64', CGO_ENABLED: '0' }
})

// ConPTY moderno (conpty.dll + OpenConsole.exe) ao lado do colmeiad.exe
execFileSync(process.execPath, [path.join(root, 'scripts', 'fetch-conpty-win.mjs')], { stdio: 'inherit' })
