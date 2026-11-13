# E2E Phase 1 (R3b rerun, after Round 4) — isolated, COLMEIA_DAEMON=1, Xvfb :97, scratch HOME/XDG (/tmp/colmeia-e2e)

Setup: out/colmeiad rebuilt (go build) + `npm run build`; electron `--remote-debugging-port`; input driver = ctypes XTest (python, no xdotool installed); screenshots via `import -window root`. Real daemon/app untouched.

- ✓ fresh HOME: app opens, daemon auto-spawned, socket in scratch rt dir, card spawns
- ✓ GUI typing (XTest keys into xterm focus): `0` (zsh wizard), `echo GUITYPE_OK_123` → echoed + `colmeia read` sees it
- ✓ CLI from card (token from card env): list, spawn, send, read
- ✓ `colmeiad --stop` while app open → bridge reconnect auto-spawns new daemon (pid changed) → both cards re-ensured, history restored in GUI (GUITYPE/GUIPERSIST_55/PERSIST_MARK_77 + `[sessão anterior encerrada]`)
- ✓ app quit → daemon pid survives; reopen OK
- ✓ version mismatch: daemon A running, app started w/ COLMEIA_DAEMON_BIN = copy w/ 1 extra byte (version differs) → dialog shown; click "Reiniciar" → old pid gone (R3-H2 wait works), new daemon up, no fatal dialog, canvas + history intact
- ✗ BUG (found, fixed in electron/daemonClient.ts): fresh install → "Falha ao conectar ao daemon: segredo do colmeia indisponível". `readSecret` waits for `~/.config/colmeia/secret`, but daemon only creates it lazily on 1st `ui.attach`/token use → deadlock. Fix: client creates it (O_EXCL, 0600, 32B hex = same format as daemon `auth.load`) before waiting. Better long-term: daemon `auth.SecretHex()` at startup (GoSvc).
- ⚠ OPEN (GoSvc/Bridge, not fixed): after mismatch restart, log showed 2nd colmeiad spawn (pid 230088): "socket ... em uso por outro backend" → exited, but `colmeiad.pid` ended with the dead 2nd pid (live daemon was 230061) → `--stop` later "não está rodando". Fix ideas: daemon writes pidfile only after socket bind succeeds (or loser must not truncate pidfile); check who double-spawns (connect() + onClose reconnect racing after --stop).
- not tested: "Manter daemon atual" button, go vet/go test (not my scope), `ui.req` screenshot

## Rerun after Round 4 fixes (bridge: single connect(); daemon: pidfile/killAll ws) — fresh scratch HOME, out/colmeiad rebuilt
- ✓ version-mismatch restart (daemon A → app w/ 1-byte-different bin, click "Reiniciar"): exactly 1 colmeiad of the isolated env before (231719) and after (232843); `colmeiad.pid` == live pid; log has 2 "pronto" lines, 0 "em uso"/"já está rodando" → double spawn + stale pidfile fixed; app on canvas, card history kept + "[sessão anterior encerrada]"
- ✓ delete workspace via GUI (create w2 w/ card, delete active w2, confirm): w2 card process dead, w2.json removed, default ws card (9b09622c) still alive + visible (N1 ok)
- fresh-HOME secret fix (previous round) still works
- not tested: "Manter daemon atual", N2/N3 race under heavy output
