import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import { isolate } from './helpers.mjs'
isolate()
const notes = await import('../../electron/notesStore.ts')

test('notes.write com baseMtime velho → conflict', () => {
  const n = notes.createNote('ws', 'c')
  const { mtime } = notes.readNote('ws', n.id)
  // alguém altera o arquivo por fora, depois do que o renderer leu
  const later = new Date(mtime + 5000)
  fs.writeFileSync(n.file, 'externo'); fs.utimesSync(n.file, later, later)
  assert.throws(() => notes.writeNote('ws', n.id, 'meu', mtime), /conflict/)
  assert.equal(fs.readFileSync(n.file, 'utf8'), 'externo')
  const fresh = notes.readNote('ws', n.id).mtime
  assert.ok(notes.writeNote('ws', n.id, 'meu', fresh).mtime > 0)
})
