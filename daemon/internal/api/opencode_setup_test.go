package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// Instruções do Colmeia vão para a configuração (prompt de sistema), nunca como mensagem inicial.
func TestOpencodeSetup(t *testing.T) {
	help2 := "  --standalone   Run with a private server\n  --prompt string"
	cmd, cfg := opencodeSetup("opencode -m opencode/hy4-flash-free", `{"theme":"x"}`, `C:\app\docs\ORCHESTRATOR.md`, help2)
	if cmd != "opencode mini --standalone -m opencode/hy4-flash-free" {
		t.Fatalf("cmd = %q", cmd)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(cfg), &got); err != nil {
		t.Fatal(err)
	}
	ins, _ := got["instructions"].([]any)
	if got["theme"] != "x" || len(ins) != 1 || ins[0] != "C:/app/docs/ORCHESTRATOR.md" {
		t.Fatalf("config = %s", cfg)
	}
	// reaplicar não duplica a instrução
	if _, again := opencodeSetup("opencode", cfg, `C:\app\docs\ORCHESTRATOR.md`, help2); strings.Count(again, "ORCHESTRATOR") != 1 {
		t.Fatalf("instrução duplicada: %s", again)
	}
	// opencode 1.x (sem --standalone, com -m): comando intacto, só instruções
	cmd1, cfg1 := opencodeSetup("opencode -m x/y", "", "/d/WORKER.md", "  -m, --model string\n  --prompt string")
	if cmd1 != "opencode -m x/y" || !strings.Contains(cfg1, `"/d/WORKER.md"`) || strings.Contains(cfg1, "model") {
		t.Fatalf("1.x: %q %s", cmd1, cfg1)
	}
}

// 1.x: "opencode mini -m X" volta para "opencode -m X"; 2.x: --standalone entra depois do "mini".
func TestOpencodeSetupMini(t *testing.T) {
	help1 := "  -m, --model string\n  --prompt string"
	help2 := "  --standalone   Run with a private server\n  --prompt string"
	if cmd, _ := opencodeSetup("opencode mini -m x/y", "", "", help1); cmd != "opencode -m x/y" {
		t.Fatalf("1.x: %q", cmd)
	}
	if cmd, _ := opencodeSetup("opencode mini -m x/y", "", "", help2); cmd != "opencode mini --standalone -m x/y" {
		t.Fatalf("2.x mini: %q", cmd)
	}
	if cmd, _ := opencodeSetup("opencode -m x/y", "", "", help2); cmd != "opencode mini --standalone -m x/y" {
		t.Fatalf("2.x legado: %q", cmd)
	}
}
