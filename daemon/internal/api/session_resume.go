package api

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"colmeia/daemon/internal/auth"
)

// Retomada de conversa: o card deve reabrir EXATAMENTE na última conversa. Um id fixo por card fica
// velho após /clear, /compact ou /new (o agente cria outra sessão), então o id da conversa atual é
// registrado em <config>/sessions/ (Claude: a statusline grava o session_id a cada atualização;
// opencode: o daemon confere a lista de sessões ao abrir). Se não der para saber qual é a última,
// o card abre uma conversa nova em vez de retomar uma antiga.

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func sessionFile(kind, cardID string) string {
	return filepath.Join(auth.ConfigDir(), "sessions", kind+"-"+safeSessionPart(cardID)+".txt")
}

func readSessionRecord(kind, cardID string) string {
	b, _ := os.ReadFile(sessionFile(kind, cardID))
	return strings.TrimSpace(string(b))
}

func writeSessionRecord(kind, cardID, id string) {
	f := sessionFile(kind, cardID)
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	tmp := f + ".tmp"
	if os.WriteFile(tmp, []byte(id), 0o644) == nil {
		_ = os.Rename(tmp, f)
	}
}

// claudeSessionChoice decide o argumento de sessão do Claude: (flag, uuid).
//   - registro existe e a conversa existe em disco → --resume registro (a última, mesmo após /clear)
//   - registro existe mas a conversa ainda não foi gravada (card sem mensagens) → --session-id registro
//   - sem registro e sem conversa do id antigo fixo → --session-id fixo (card novo)
//   - sem registro mas o id fixo já tem conversa (instalação antiga: pode estar velha) → conversa nova
//
// here = a conversa existe na pasta de projeto do cwd do card (único lugar onde --resume acha);
// anywhere = existe em qualquer pasta (conversa de outro cwd: não dá para retomar nem recriar o id → nova).
func claudeSessionChoice(cardID string, here, anywhere func(string) bool, fresh func() string) (flag, id string) {
	rec := readSessionRecord("claude", cardID)
	if uuidRe.MatchString(rec) {
		if here(rec) {
			return "--resume", rec
		}
		if anywhere(rec) {
			return "--session-id", fresh()
		}
		return "--session-id", rec
	}
	pinned := cardSessionUUID(cardID)
	if !anywhere(pinned) {
		return "--session-id", pinned
	}
	return "--session-id", fresh()
}

// freshClaudeUUID: uuid v4 novo e único (derivado do card + relógio).
func freshClaudeUUID(cardID string) string {
	return cardSessionUUID(cardID + "#" + strconv.FormatInt(time.Now().UnixNano(), 36))
}

const ocSessionLimit = 100

type ocSession struct {
	ID      string `json:"id"`
	Updated int64  `json:"updated"`
}

// chooseOpencodeSession: cur = sessão registrada do card; list = sessões do projeto, mais recente primeiro.
// Retoma cur só se ela ainda não existe (criar) ou continua a mais recente; do contrário (outra sessão
// mais nova — /new, /clear — ou lista indisponível) abre uma conversa nova.
func chooseOpencodeSession(cardID, cur string, list []ocSession, listed bool, now time.Time, others map[string]bool) string {
	fresh := "ses_colmeia_" + safeSessionPart(cardID) + "_" + strconv.FormatInt(now.Unix(), 36)
	if !listed {
		return fresh
	}
	rawLen := len(list)
	kept := make([]ocSession, 0, len(list))
	for _, s := range list { // sessões de OUTROS cards do mesmo projeto não contam
		if !others[s.ID] {
			kept = append(kept, s)
		}
	}
	list = kept
	sort.SliceStable(list, func(i, j int) bool { return list[i].Updated > list[j].Updated })
	found := false
	for _, s := range list {
		if s.ID == cur {
			found = true
		}
	}
	if !found {
		// lista cortada em ocSessionLimit: cur pode existir fora dela (antiga) → não retoma às cegas
		if rawLen >= ocSessionLimit {
			return fresh
		}
		return cur
	}
	if len(list) > 0 && list[0].ID == cur {
		return cur
	}
	return fresh
}

func opencodeSessionList(cwd string) ([]ocSession, bool) {
	bin, err := exec.LookPath("opencode")
	if err != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, bin, "session", "list", "--format", "json", "-n", strconv.Itoa(ocSessionLimit))
	c.Dir = cwd
	hideWindow(c)
	out, err := c.Output()
	if err != nil {
		return nil, false
	}
	var l []ocSession
	if json.Unmarshal(out, &l) != nil {
		return nil, false
	}
	return l, true
}

// opencodeSessionFor: id da sessão que o card deve abrir (e registra a escolha).
func opencodeSessionFor(cardID, cwd string) string {
	cur := readSessionRecord("opencode", cardID)
	if cur == "" {
		cur = "ses_colmeia_" + safeSessionPart(cardID)
	}
	list, ok := opencodeSessionList(cwd)
	others := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(auth.ConfigDir(), "sessions", "opencode-*.txt"))
	own := sessionFile("opencode", cardID)
	for _, f := range files {
		if f != own {
			b, _ := os.ReadFile(f)
			if id := strings.TrimSpace(string(b)); id != "" {
				others[id] = true
			}
		}
	}
	id := chooseOpencodeSession(cardID, cur, list, ok, time.Now(), others)
	writeSessionRecord("opencode", cardID, id)
	return id
}
