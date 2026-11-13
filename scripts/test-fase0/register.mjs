// Resolve imports relativos sem extensão para .ts (Node já remove os tipos).
import { registerHooks } from 'node:module'
import fs from 'node:fs'
import { fileURLToPath } from 'node:url'
registerHooks({
  resolve(spec, ctx, next) {
    if (spec.startsWith('.') && !/\.\w+$/.test(spec) && ctx.parentURL?.startsWith('file:')) {
      const url = new URL(spec + '.ts', ctx.parentURL)
      if (fs.existsSync(fileURLToPath(url))) return { url: url.href, shortCircuit: true }
    }
    return next(spec, ctx)
  }
})
