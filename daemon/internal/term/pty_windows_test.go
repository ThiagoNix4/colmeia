//go:build windows

package term

import (
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBlankControlCells(t *testing.T) {
	b := []byte("\x1b[?2004h\x0f\ba\x1b[K\r\n\x0e\x01\x7f\té\a")
	blankControlCells(b)
	if want := "\x1b[?2004h \ba\x1b[K\r\n   \té\a"; string(b) != want {
		t.Fatalf("got %q, want %q", b, want)
	}
}

// Conhost antigo + página de código UTF-8: o SI ocupa uma célula. A tela lida pelo daemon
// (mesmos bytes que vão ao xterm.js) precisa manter a coluna.
func TestLegacyConhostControlCell(t *testing.T) {
	if !legacyConhost || modernConpty() {
		t.Skip("conhost novo ou ConPTY moderno: SI não vira célula")
	}
	m := New(Options{HistoryDir: t.TempDir()})
	t.Cleanup(func() { _ = m.Kill("c1") })
	script := "[Console]::OutputEncoding=[Text.Encoding]::UTF8; [Console]::WriteLine('AB' + [char]15 + 'CD|fim'); Start-Sleep 20"
	if _, _, err := m.Ensure(SpawnReq{ID: "c1", Command: "powershell.exe", Args: []string{"-NoLogo", "-NoProfile", "-Command", script}, Cwd: t.TempDir(), Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	out := ""
	for i := 0; i < 100; i++ {
		if out, _ = m.Read("c1", 20); strings.Contains(out, "AB CD|fim") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("SI não virou espaço: %q", out)
}

func TestSplitPaced(t *testing.T) {
	text := strings.Repeat("ação 日本 ", 100) // multibyte: nenhum corte pode partir uma runa
	parts := splitPaced(text, pacedChunk)
	for _, p := range parts {
		if len(p) > pacedChunk || !utf8.ValidString(p) {
			t.Fatalf("pedaço inválido (%d bytes): %q", len(p), p)
		}
	}
	if strings.Join(parts, "") != text || len(parts) < 2 {
		t.Fatalf("texto alterado ou não dividido: %d pedaços", len(parts))
	}
}

// ConPTY moderno: envio longo e multilinha em bracketed paste chega inteiro, sem o envio em pedaços.
func TestModernSendIntact(t *testing.T) {
	if !modernConpty() {
		t.Skip("sem conpty.dll ao lado do binário (COLMEIA_CONPTY_DIR)")
	}
	m := New(Options{HistoryDir: t.TempDir()})
	t.Cleanup(func() { _ = m.Kill("c2") })
	js := `process.stdout.write('\x1b[?2004h');process.stdin.setRawMode(true);let n=0,t;process.stdin.on('data',d=>{n+=d.length;clearTimeout(t);t=setTimeout(()=>console.log('GOT='+n),1200)});setTimeout(()=>process.exit(),30000)`
	if _, _, err := m.Ensure(SpawnReq{ID: "c2", Command: "node", Args: []string{"-e", js}, Cwd: t.TempDir(), Cols: 120, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	text := strings.Repeat("x", 300) + "\n" + strings.Repeat("y", 300)
	m.Send("c2", text, false)
	want := "GOT=" + strconv.Itoa(len(text)+len("\x1b[200~")+len("\x1b[201~"))
	for i := 0; i < 100; i++ {
		if out, _ := m.Read("c2", 20); strings.Contains(out, want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, _ := m.Read("c2", 20)
	t.Fatalf("esperava %s, tela: %q", want, out)
}
