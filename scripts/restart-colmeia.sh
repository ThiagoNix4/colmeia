#!/usr/bin/env bash
# Reinicia o Colmeia carregando o build novo.
#
# Precisa rodar DESACOPLADO: quem chama este script normalmente está dentro de um card
# do próprio Colmeia, e matar o app mataria o shell que está executando o script antes
# de ele conseguir subir a instância nova. Por isso o script se re-executa com setsid.
set -u

APP="$(cd "$(dirname "$0")/.." && pwd)"
LOG="$APP/.tasks/restart.log"

if [ "${1:-}" != "--detached" ]; then
  mkdir -p "$APP/.tasks"
  setsid nohup "$0" --detached >>"$LOG" 2>&1 &
  echo "reinício disparado em segundo plano; log: $LOG"
  exit 0
fi

echo "=== $(date '+%F %T') reiniciando ==="
cd "$APP" || exit 1

# 1. build atualizado; se falhar, não derruba o app que está rodando
if ! npm run build; then
  echo "build FALHOU — mantendo a instância atual de pé"
  exit 1
fi

# 2. derruba só o Electron: o daemon e os cards seguem vivos (versão/cli/docs diferentes → o app pergunta se reinicia)
pkill -f "$APP/node_modules/electron" 2>/dev/null

# 3. espera soltar o single-instance lock antes de subir a nova
for _ in $(seq 1 60); do
  pgrep -f "$APP/node_modules/electron/dist/electron" >/dev/null || break
  sleep 0.25
done

# 4. sobe de novo, também desacoplado
echo "subindo npm run dev..."
exec npm run dev
