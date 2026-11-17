package api

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"colmeia/daemon/internal/auth"
)

// cardSessionUUID: UUID estável por card (e por instância do Colmeia), no formato v4.
func cardSessionUUID(cardID string) string {
	h := sha256.Sum256([]byte("colmeia-session:" + auth.Instance() + ":" + cardID))
	h[6] = h[6]&0x0f | 0x40 // versão 4
	h[8] = h[8]&0x3f | 0x80 // variante RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// claudeSessionExists: o Claude Code grava cada conversa em <config>/projects/<pasta>/<uuid>.jsonl.
func claudeSessionExists(uuid string) bool {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, ".claude")
	}
	m, _ := filepath.Glob(filepath.Join(dir, "projects", "*", uuid+".jsonl"))
	return len(m) > 0
}

// claudeSessionArg: cada card do Claude tem sua conversa fixa. Primeira vez: --session-id <uuid>
// (cria com esse id); depois: --resume <uuid> (o card reabre na mesma conversa, com o histórico).
// Comando que já escolhe a sessão, ou não é interativo (-p), fica como está.
func claudeSessionArg(cmd, cardID string) string {
	t := strings.TrimLeft(cmd, " \t\r\n")
	f := strings.Fields(t)
	if len(f) == 0 || f[0] != "claude" {
		return cmd
	}
	for _, x := range f[1:] {
		switch {
		case x == "-r" || x == "-c" || x == "-p" || x == "--print" || x == "--continue",
			strings.HasPrefix(x, "--resume"), strings.HasPrefix(x, "--session-id"):
			return cmd
		}
	}
	if cardID == "" {
		return cmd
	}
	uuid := cardSessionUUID(cardID)
	flag := " --session-id "
	if claudeSessionExists(uuid) {
		flag = " --resume "
	}
	return "claude" + flag + uuid + strings.TrimPrefix(t, "claude")
}
