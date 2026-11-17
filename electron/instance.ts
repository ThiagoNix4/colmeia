// Identidade desta instalação do Colmeia (instância).
//
// Tudo o que é "por app" deriva daqui: pasta de config (~/.config/<id>), pipe/socket do daemon,
// pasta de dados (~/.local/share/<id>), perfil do Electron (userData) e o lock de instância única.
// Uma variante (fork) só precisa de outro "name" no package.json para ganhar um daemon e dados
// próprios; sem isso as duas cópias se conectam ao mesmo daemon e dividem workspaces e cards.
// $COLMEIA_INSTANCE sobrepõe o valor (útil para testar duas instâncias a partir do mesmo código).
import { app } from 'electron'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

/** Mesma regra do daemon (auth.Instance): minúsculas, [a-z0-9._-]; vazio → "colmeia". */
export function sanitizeInstance(v: string | undefined): string {
  const s = (v ?? '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]/g, '')
    .replace(/^\.+|\.+$/g, '')
  return s || 'colmeia'
}

function readPkg(): { name?: string; build?: { productName?: string } } {
  for (const dir of [app.getAppPath(), process.cwd()]) {
    try {
      return JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8'))
    } catch {
      /* tenta o próximo */
    }
  }
  return {}
}

const pkg = readPkg()

// Aberto de dentro de um card (ex.: uma IA rodando `npm run dev` num terminal do Colmeia):
// o ambiente do card aponta para o daemon do OUTRO app. Descarta essas variáveis, senão esta
// cópia se anexaria ao daemon de quem a abriu.
const insideCard = !!process.env.COLMEIA_CARD_ID
if (insideCard) {
  for (const k of ['COLMEIA_INSTANCE', 'COLMEIA_CARD_ID', 'COLMEIA_CARD_TITLE', 'COLMEIA_SOCK', 'COLMEIA_TOKEN', 'COLMEIA_NODE']) {
    delete process.env[k]
  }
}

/** Identificador da instância (ex.: "colmeia", "colmeia-dev"). */
export const INSTANCE = sanitizeInstance(process.env.COLMEIA_INSTANCE || pkg.name)
/** Nome exibido (título da janela, diálogos). */
export const PRODUCT_NAME = pkg.build?.productName || 'Colmeia'

// daemon, cards, CLI e statusline herdam esta variável
process.env.COLMEIA_INSTANCE = INSTANCE

export const configDir = () => path.join(process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'), INSTANCE)
export const dataDir = () => path.join(process.env.XDG_DATA_HOME || path.join(os.homedir(), '.local/share'), INSTANCE)

/** win32: named pipe por usuário e instância; unix: socket em XDG_RUNTIME_DIR. */
export const daemonSocketPath = () =>
  process.env.COLMEIA_SOCK_PATH ||
  (process.platform === 'win32'
    ? `\\\\.\\pipe\\${INSTANCE}-${os.userInfo().username}`
    : path.join(process.env.XDG_RUNTIME_DIR || os.tmpdir(), `${INSTANCE}.sock`))
