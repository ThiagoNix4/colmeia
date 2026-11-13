#!/usr/bin/env bash
# Instala o Colmeia para o usuário atual (idempotente).
set -euo pipefail

ROOT="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
BIN="$HOME/.local/bin"
APPS="$HOME/.local/share/applications"

echo "→ Compilando…"
(cd "$ROOT" && npm run build --silent)

echo "→ Instalando a CLI colmeia em $BIN"
mkdir -p "$BIN"
ln -sf "$ROOT/cli/colmeia" "$BIN/colmeia"
chmod +x "$ROOT/cli/colmeia" "$ROOT/cli/colmeia.mjs" "$ROOT/scripts/colmeia"
case ":$PATH:" in
  *":$BIN:"*) ;;
  *) echo "  ⚠  $BIN não está no PATH. Adicione ao ~/.zshrc:  export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

echo "→ Instalando ícone"
HICOLOR="$HOME/.local/share/icons/hicolor"
ICONS="$HICOLOR/scalable/apps"
mkdir -p "$ICONS"
cp "$ROOT/assets/icon.svg" "$ICONS/colmeia.svg"

# PNGs em vários tamanhos: rsvg-convert, com magick como fallback
render_png() { # $1=tamanho $2=saída
  if command -v rsvg-convert >/dev/null; then
    rsvg-convert -w "$1" -h "$1" "$ROOT/assets/icon.svg" -o "$2"
  else
    magick -background none -density 384 "$ROOT/assets/icon.svg" -resize "$1x$1" "$2"
  fi
}
if command -v rsvg-convert >/dev/null || command -v magick >/dev/null; then
  for s in 16 32 48 64 128 256 512; do
    d="$HICOLOR/${s}x${s}/apps"
    mkdir -p "$d"
    render_png "$s" "$d/colmeia.png" || echo "  ⚠  falha ao gerar PNG ${s}x${s}"
  done
  command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q -f -t "$HICOLOR" || true
else
  echo "  ⚠  rsvg-convert e magick ausentes; PNGs não gerados (só SVG instalado)"
fi

echo "→ Criando atalho no menu de aplicativos"
mkdir -p "$APPS"
cat > "$APPS/colmeia.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=Colmeia
Comment=Canvas espacial de terminais para orquestrar IAs
Exec=$ROOT/scripts/colmeia
Icon=colmeia
Terminal=false
Categories=Development;
Keywords=terminal;ai;ia;claude;opencode;canvas;
StartupWMClass=colmeia
DESKTOP
command -v update-desktop-database >/dev/null && update-desktop-database "$APPS" || true

echo "✓ Pronto. Abra \"Colmeia\" pelo menu do GNOME ou rode: $ROOT/scripts/colmeia"
