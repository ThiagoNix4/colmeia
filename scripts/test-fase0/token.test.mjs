import { test } from 'node:test'
import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { isolate, fakeCtx } from './helpers.mjs'
const env0 = isolate()
const { startApiServer } = await import('../../electron/apiServer.ts')
const { tokenFor } = await import('../../electron/auth.ts')
const run = promisify(execFile)
const cli = new URL('../../cli/colmeia.mjs', import.meta.url).pathname

test('CLI: from sem token / token ruim rejeitado; token válido ok', async () => {
  const server = startApiServer(env0.sock, fakeCtx())
  await new Promise((r) => server.once('listening', r))
  const base = { PATH: process.env.PATH, COLMEIA_SOCK: env0.sock }
  const go = (extra) => run('node', [cli, 'list', '--json'], { env: { ...base, ...extra } }).then(
    (r) => ({ ok: true, out: r.stdout }), (e) => ({ ok: false, err: e.stderr }))
  const none = await go({ COLMEIA_CARD_ID: 'a' })
  const bad = await go({ COLMEIA_CARD_ID: 'a', COLMEIA_TOKEN: tokenFor('b') }) // token de outro card
  const good = await go({ COLMEIA_CARD_ID: 'a', COLMEIA_TOKEN: tokenFor('a') })
  const user = await go({})
  server.close()
  assert.equal(none.ok, false); assert.match(none.err, /token/)
  assert.equal(bad.ok, false); assert.match(bad.err, /token/)
  assert.equal(good.ok, true)
  assert.equal(user.ok, true)
})
