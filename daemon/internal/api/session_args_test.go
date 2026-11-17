package api

import (
	"regexp"
	"strings"
	"testing"
)

func TestOpencodeSessionArg(t *testing.T) {
	if got := opencodeSessionArg("opencode --standalone", "ab12cd34"); got != "opencode --session ses_colmeia_ab12cd34 --standalone" {
		t.Fatalf("got %q", got)
	}
	for _, c := range []string{"opencode -c", "opencode --session ses_x", "opencode -s ses_x"} {
		if got := opencodeSessionArg(c, "ab12cd34"); got != c {
			t.Fatalf("%q alterado para %q", c, got)
		}
	}
}

func TestClaudeSessionArg(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir()) // sem conversas gravadas → --session-id
	u := cardSessionUUID("ab12cd34")
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(u) {
		t.Fatalf("uuid inválido: %s", u)
	}
	if u != cardSessionUUID("ab12cd34") || u == cardSessionUUID("ffff0000") {
		t.Fatal("uuid precisa ser estável por card e distinto entre cards")
	}
	got := claudeSessionArg("claude --settings 'x' --effort medium", "ab12cd34")
	if got != "claude --session-id "+u+" --settings 'x' --effort medium" {
		t.Fatalf("got %q", got)
	}
	for _, c := range []string{"claude --resume abc", "claude -c", "claude -p hi", "opencode"} {
		if got := claudeSessionArg(c, "ab12cd34"); got != c {
			t.Fatalf("%q alterado para %q", c, got)
		}
	}
	if !strings.HasPrefix(claudeSessionArg("claude", "ab12cd34"), "claude --session-id ") {
		t.Fatal("claude puro deveria ganhar --session-id")
	}
}
