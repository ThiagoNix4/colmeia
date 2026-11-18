package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOpencodeSessionArg(t *testing.T) {
	pick := func() string { return "ses_colmeia_ab12cd34" }
	for in, want := range map[string]string{
		"opencode --standalone":                "opencode --session ses_colmeia_ab12cd34 --standalone",
		"opencode mini --standalone -m x/y":    "opencode mini --session ses_colmeia_ab12cd34 --standalone -m x/y",
		"opencode mini -m opencode-go/gpt-6-l": "opencode mini --session ses_colmeia_ab12cd34 -m opencode-go/gpt-6-l",
	} {
		if got := opencodeSessionArg(in, pick); got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	for _, c := range []string{"opencode run hi", "opencode -c", "opencode --session ses_x", "opencode -s ses_x", "opencode mini -c"} {
		if got := opencodeSessionArg(c, pick); got != c {
			t.Fatalf("%q alterado para %q", c, got)
		}
	}
}

func TestChooseOpencodeSession(t *testing.T) {
	now := time.Unix(1000, 0)
	cur := "ses_colmeia_c1"
	if got := chooseOpencodeSession("c1", cur, nil, true, now, nil); got != cur {
		t.Fatalf("sessão ainda não existe: deve criar com %s, veio %s", cur, got)
	}
	if got := chooseOpencodeSession("c1", cur, []ocSession{{ID: "x", Updated: 1}, {ID: cur, Updated: 2}}, true, now, nil); got != cur {
		t.Fatalf("mais recente: deve retomar, veio %s", got)
	}
	got := chooseOpencodeSession("c1", cur, []ocSession{{ID: cur, Updated: 1}, {ID: "novo", Updated: 2}}, true, now, nil)
	if got == cur || !strings.HasPrefix(got, "ses_colmeia_c1_") {
		t.Fatalf("há sessão mais nova: deve abrir conversa nova, veio %s", got)
	}
	if got := chooseOpencodeSession("c1", cur, []ocSession{{ID: cur}}, false, now, nil); got == cur {
		t.Fatal("lista indisponível: não retomar às cegas")
	}
}

func TestClaudeSessionArg(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	u := cardSessionUUID("ab12cd34")
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(u) {
		t.Fatalf("uuid inválido: %s", u)
	}
	if u != cardSessionUUID("ab12cd34") || u == cardSessionUUID("ffff0000") {
		t.Fatal("uuid precisa ser estável por card e distinto entre cards")
	}
	got := claudeSessionArg("claude --settings 'x' --effort medium", "ab12cd34", "/w")
	if got != "claude --session-id "+u+" --settings 'x' --effort medium" {
		t.Fatalf("got %q", got)
	}
	for _, c := range []string{"claude --resume abc", "claude -c", "claude -p hi", "opencode"} {
		if got := claudeSessionArg(c, "ab12cd34", "/w"); got != c {
			t.Fatalf("%q alterado para %q", c, got)
		}
	}
}

func TestClaudeSessionOtherCwd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeSessionRecord("claude", "c1", "22222222-2222-4222-8222-222222222222")
	f, id := claudeSessionChoice("c1", func(string) bool { return false }, func(string) bool { return true }, func() string { return "fresh" })
	if f != "--session-id" || id != "fresh" {
		t.Fatalf("conversa de outro cwd: %s %s", f, id)
	}
	if claudeProjectDirName("C:"+string(rune(92))+"Users"+string(rune(92))+"a b") != "C--Users-a-b" {
		t.Fatal("nome da pasta do projeto")
	}
	full := make([]ocSession, ocSessionLimit)
	for i := range full {
		full[i] = ocSession{ID: "o" + strconv.Itoa(i), Updated: int64(i)}
	}
	if got := chooseOpencodeSession("c1", "ses_colmeia_c1", full, true, time.Unix(1, 0), nil); got == "ses_colmeia_c1" {
		t.Fatal("lista cheia sem a sessão: não retomar")
	}
}

func TestOpencodeSharedCwd(t *testing.T) {
	list := []ocSession{{ID: "ses_colmeia_b", Updated: 9}, {ID: "ses_colmeia_a", Updated: 5}}
	if got := chooseOpencodeSession("a", "ses_colmeia_a", list, true, time.Unix(1, 0), map[string]bool{"ses_colmeia_b": true}); got != "ses_colmeia_a" {
		t.Fatalf("sessão de outro card não deve contar: %s", got)
	}
}

func TestClaudeSessionExistsCleanAndCase(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	cwd := filepath.Join(t.TempDir(), "proj")
	dir := filepath.Join(cfg, "projects", strings.ToUpper(claudeProjectDirName(cwd)))
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "u.jsonl"), nil, 0o644)
	if !claudeSessionExists("u", cwd+string(filepath.Separator)) {
		t.Fatal("cwd com barra final / diferença de caixa deveria achar")
	}
}

func TestClaudeSessionChoice(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	card := "ab12cd34"
	pinned := cardSessionUUID(card)
	fresh := func() string { return "11111111-1111-4111-8111-111111111111" }
	has := map[string]bool{}
	exists := func(id string) bool { return has[id] }

	// card novo → id fixo
	if f, id := claudeSessionChoice(card, exists, exists, fresh); f != "--session-id" || id != pinned {
		t.Fatalf("novo: %s %s", f, id)
	}
	// instalação antiga: conversa fixa existe, sem registro → conversa nova (não retoma a velha)
	has[pinned] = true
	if f, id := claudeSessionChoice(card, exists, exists, fresh); f != "--session-id" || id != fresh() {
		t.Fatalf("legado: %s %s", f, id)
	}
	// /clear: statusline registrou a nova conversa → retoma ESTA, não a fixa
	cleared := "22222222-2222-4222-8222-222222222222"
	writeSessionRecord("claude", card, cleared)
	has[cleared] = true
	if f, id := claudeSessionChoice(card, exists, exists, fresh); f != "--resume" || id != cleared {
		t.Fatalf("após /clear: %s %s", f, id)
	}
	// registro sem conversa gravada ainda (nenhuma mensagem) → reaproveita o id
	delete(has, cleared)
	if f, id := claudeSessionChoice(card, exists, exists, fresh); f != "--session-id" || id != cleared {
		t.Fatalf("sem mensagens: %s %s", f, id)
	}
}
