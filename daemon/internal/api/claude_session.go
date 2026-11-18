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

func claudeConfigDir() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, ".claude")
	}
	return dir
}

// claudeProjectDirName: o Claude Code nomeia a pasta do projeto trocando todo caractere não alfanumérico por "-".
func claudeProjectDirName(cwd string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, cwd)
}

// claudeSessionExists: o Claude Code grava cada conversa em <config>/projects/<pasta>/<uuid>.jsonl.
// "--resume <id>" só enxerga a pasta do cwd atual: aqui confere só ela.
func claudeSessionExists(uuid, cwd string) bool {
	root := filepath.Join(claudeConfigDir(), "projects")
	name := claudeProjectDirName(filepath.Clean(cwd))
	if _, err := os.Stat(filepath.Join(root, name, uuid+".jsonl")); err == nil {
		return true
	}
	// o Claude encurta nomes longos (>200) e acrescenta hash; também tolera diferença de caixa (Windows)
	ents, _ := os.ReadDir(root)
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		n := e.Name()
		match := strings.EqualFold(n, name)
		if !match && len(name) > 200 {
			match = len(n) >= 200 && strings.EqualFold(n[:200], name[:200])
		}
		if match {
			if _, err := os.Stat(filepath.Join(root, n, uuid+".jsonl")); err == nil {
				return true
			}
		}
	}
	return false
}

// claudeSessionExistsAnywhere: a conversa existe em alguma pasta de projeto (mesmo de outro cwd).
func claudeSessionExistsAnywhere(uuid string) bool {
	m, _ := filepath.Glob(filepath.Join(claudeConfigDir(), "projects", "*", uuid+".jsonl"))
	return len(m) > 0
}

// claudeSessionArg: cada card do Claude reabre na ÚLTIMA conversa (ver claudeSessionChoice).
// Comando que já escolhe a sessão, ou não é interativo (-p), fica como está.
func claudeSessionArg(cmd, cardID, cwd string) string {
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
	flag, uuid := claudeSessionChoice(cardID, func(id string) bool { return claudeSessionExists(id, cwd) }, claudeSessionExistsAnywhere, func() string { return freshClaudeUUID(cardID) })
	writeSessionRecord("claude", cardID, uuid) // a statusline atualiza (após /clear, /compact...)
	return "claude " + flag + " " + uuid + strings.TrimPrefix(t, "claude")
}
