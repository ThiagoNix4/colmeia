package api

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// opencode 2.x tirou o -m/--model do TUI ("Unrecognized flag: -m"): o modelo passou a vir da configuração,
// e o cliente só lê OPENCODE_CONFIG_CONTENT quando sobe o próprio servidor (--standalone).
// Presets "opencode -m X ..." são traduzidos na hora do spawn; opencode 1.x segue com o comando original.

var opencodeModelRe = regexp.MustCompile(`(?s)^(\s*opencode)\s+(?:-m|--model)(?:\s+|=)(\S+)(.*)$`)

// opencodeModelArg: separa "opencode -m X resto" em ("opencode --standalone resto", "X").
func opencodeModelArg(cmd string) (rest, model string, ok bool) {
	m := opencodeModelRe.FindStringSubmatch(cmd)
	if m == nil {
		return cmd, "", false
	}
	model = strings.Trim(m[2], `'"`)
	if model == "" {
		return cmd, "", false
	}
	return m[1] + " --standalone" + m[3], model, true
}

// opencodeTUILacksModel: a ajuda do TUI lista --standalone e não lista mais --model.
func opencodeTUILacksModel(help string) bool {
	return strings.Contains(help, "--standalone") && !strings.Contains(help, "--model")
}

var (
	helpMu    sync.Mutex
	helpCache = map[string]string{} // caminho+mtime do opencode → saída do --help
)

// opencodeHelp: saída de "opencode --help"; vazio se não achar ou travar. Roda uma vez por
// versão instalada (cache por caminho+data do arquivo) e sem janela de console no Windows.
func opencodeHelp() string {
	bin, err := exec.LookPath("opencode")
	if err != nil {
		return ""
	}
	key := bin
	if st, err := os.Stat(bin); err == nil {
		key += "|" + st.ModTime().String()
	}
	helpMu.Lock()
	defer helpMu.Unlock()
	if out, ok := helpCache[key]; ok {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, bin, "--help")
	hideWindow(c)
	out, _ := c.CombinedOutput()
	helpCache[key] = string(out)
	return string(out)
}

// opencodeSessionArg: cada card tem sua conversa fixa no opencode ("--session" cria se não existir
// e continua se existir; o id precisa começar com "ses"). Ao reabrir o app, o card volta para a
// mesma conversa. Comando que já escolhe sessão (--session/-s/--continue/-c) fica como está.
func opencodeSessionArg(cmd, cardID string) string {
	for _, f := range strings.Fields(cmd)[1:] {
		if f == "-s" || f == "-c" || f == "--continue" || strings.HasPrefix(f, "--session") {
			return cmd
		}
	}
	id := safeSessionPart(cardID)
	if id == "" {
		return cmd
	}
	i := strings.Index(cmd, "opencode") + len("opencode")
	return cmd[:i] + " --session ses_colmeia_" + id + cmd[i:]
}

func safeSessionPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isOpencodeCmd: o comando do preset abre o opencode (primeira palavra).
func isOpencodeCmd(cmd string) bool {
	f := strings.Fields(cmd)
	return len(f) > 0 && f[0] == "opencode"
}

// opencodeSetup prepara um card do opencode:
//   - instruções do Colmeia (WORKER.md / ORCHESTRATOR.md) entram como "instructions" da configuração,
//     ou seja, no prompt de sistema. Antes iam por --prompt, que vira a primeira mensagem do chat:
//     aparecia no card e gastava tokens só por abrir o terminal.
//   - opencode 2.x: modelo também pela configuração (o TUI não aceita mais -m).
//   - 2.x só lê OPENCODE_CONFIG_CONTENT com servidor próprio: acrescenta --standalone quando existe.
//
// Devolve o comando ajustado e o novo OPENCODE_CONFIG_CONTENT.
func opencodeSetup(cmd, current, rulesFile, help string) (string, string) {
	model := ""
	if rest, m, ok := opencodeModelArg(cmd); ok && opencodeTUILacksModel(help) {
		cmd, model = rest, m
	}
	if strings.Contains(help, "--standalone") && !strings.Contains(cmd, "--standalone") {
		i := strings.Index(cmd, "opencode") + len("opencode")
		cmd = cmd[:i] + " --standalone" + cmd[i:]
	}
	cfg := map[string]any{}
	if strings.TrimSpace(current) != "" {
		if err := json.Unmarshal([]byte(current), &cfg); err != nil {
			cfg = map[string]any{}
		}
	}
	if model != "" {
		cfg["model"] = model
	}
	if rulesFile != "" {
		f := filepath.ToSlash(rulesFile) // "instructions" aceita globs: barra invertida seria escape
		var list []any
		if l, ok := cfg["instructions"].([]any); ok {
			list = l
		}
		dup := false
		for _, x := range list {
			if x == f {
				dup = true
			}
		}
		if !dup {
			list = append(list, f)
		}
		cfg["instructions"] = list
	}
	b, _ := json.Marshal(cfg)
	return cmd, string(b)
}

// opencodeConfigWithModel: põe "model" no JSON de OPENCODE_CONFIG_CONTENT, mantendo o que o usuário já tinha.
func opencodeConfigWithModel(current, model string) string {
	cfg := map[string]any{}
	if strings.TrimSpace(current) != "" {
		if err := json.Unmarshal([]byte(current), &cfg); err != nil {
			cfg = map[string]any{}
		}
	}
	cfg["model"] = model
	b, _ := json.Marshal(cfg)
	return string(b)
}
