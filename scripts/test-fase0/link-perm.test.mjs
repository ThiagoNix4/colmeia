import { test } from 'node:test'
import assert from 'node:assert/strict'
import { isolate, fakeCtx, rpc } from './helpers.mjs'
const env0 = isolate()
const { startApiServer } = await import('../../electron/apiServer.ts')
const { tokenFor } = await import('../../electron/auth.ts')
const notes = await import('../../electron/notesStore.ts')
const links = await import('../../electron/links.ts')

test('link sem controle (self-grant) e nota↔nota rejeitados', async () => {
  const server = startApiServer(env0.sock, fakeCtx('ws'))
  await new Promise((r) => server.once('listening', r))
  const as = (id) => ({ from: id, token: tokenFor(id) })
  // a tenta se dar acesso a b sem aresta a→b
  const r1 = await rpc(env0.sock, 'link', { ...as('a'), a: 'a', b: 'b' })
  assert.equal(r1.ok, false); assert.match(r1.error, /sem link/)
  assert.equal(links.controls('a', 'b'), false)
  // nota↔nota (usuário, sem from) rejeitada
  const n1 = notes.createNote('ws', 'n1'), n2 = notes.createNote('ws', 'n2')
  const r2 = await rpc(env0.sock, 'link', { a: n1.id, b: n2.id })
  assert.equal(r2.ok, false); assert.match(r2.error, /nota/)
  // com aresta a→b, a passa a poder comandar b
  links.addEdge('a', 'b')
  const r3 = await rpc(env0.sock, 'send', { ...as('a'), id: 'b', text: 'x' })
  assert.equal(r3.ok, true)
  server.close()
})
