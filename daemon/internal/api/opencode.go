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

// opencode 2.x tirou o -m/--model do TUI padrão ("Unrecognized flag: -m"): TUI com modelo é "opencode mini -m X".
// Presets antigos "opencode -m X ..." são reescritos na hora do spawn; opencode 1.x segue com o comando original.
// O cliente só lê OPENCODE_CONFIG_CONTENT (instruções) quando sobe o próprio servidor (--standalone).

var opencodeModelRe = regexp.MustCompile(`(?s)^(\s*opencode)\s+(?:-m|--model)(?:\s+|=)(\S+)(.*)$`)

var opencodeMiniRe = regexp.MustCompile(`^(\s*opencode)\s+mini\s+(?:-m|--model)(?:\s+|=)`)

// opencodeModelArg: reescreve "opencode -m X resto" em ("opencode mini -m X resto", "X").
func opencodeModelArg(cmd string) (rest, model string, ok bool) {
	m := opencodeModelRe.FindStringSubmatch(cmd)
	if m == nil {
		return cmd, "", false
	}
	model = strings.Trim(m[2], `'"`)
	if model == "" {
		return cmd, "", false
	}
	return m[1] + " mini -m " + m[2] + m[3], model, true
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

// opencodeSessionArg: cada card tem sua conversa no opencode ("--session" cria se não existir e
// continua se existir). pick() devolve o id da sessão do card (ver opencodeSessionFor); só é chamado
// quando o comando aceita --session. TUI padrão e "mini" aceitam; outros subcomandos (run...) não
// ("Unrecognized flag: --session"). Comando que já escolhe sessão (--session/-s/--continue/-c) fica como está.
func opencodeSessionArg(cmd string, pick func() string) string {
	fields := strings.Fields(cmd)
	if len(fields) < 2 {
		fields = append(fields, "")
	}
	mini := fields[1] == "mini"
	if !strings.HasPrefix(fields[1], "-") && fields[1] != "" && !mini {
		return cmd
	}
	for _, f := range fields[1:] {
		if f == "-s" || f == "-c" || f == "--continue" || strings.HasPrefix(f, "--session") {
			return cmd
		}
	}
	id := safeSessionPart(pick())
	if id == "" {
		return cmd
	}
	i := strings.Index(cmd, "opencode") + len("opencode")
	if mini {
		i = strings.Index(cmd[i:], "mini") + i + len("mini")
	}
	return cmd[:i] + " --session " + id + cmd[i:]
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
//   - opencode 2.x: "opencode -m X" vira "opencode mini -m X" (o TUI padrão não aceita mais -m).
//   - 2.x só lê OPENCODE_CONFIG_CONTENT com servidor próprio: acrescenta --standalone quando existe.
//
// Devolve o comando ajustado e o novo OPENCODE_CONFIG_CONTENT.
func opencodeSetup(cmd, current, rulesFile, help string) (string, string) {
	if rest, _, ok := opencodeModelArg(cmd); ok && opencodeTUILacksModel(help) {
		cmd = rest
	} else if help != "" && !opencodeTUILacksModel(help) {
		// 1.x: "mini" seria lido como caminho do projeto; volta para "opencode -m X"
		cmd = opencodeMiniRe.ReplaceAllString(cmd, "$1 -m ")
	}
	if strings.Contains(help, "--standalone") && !strings.Contains(cmd, "--standalone") {
		i := strings.Index(cmd, "opencode") + len("opencode")
		if m := opencodeMiniRe.FindString(cmd); m != "" || strings.HasPrefix(strings.TrimSpace(cmd[i:]), "mini ") {
			// depois do "mini": "opencode mini --standalone -m X"
			j := strings.Index(cmd[i:], "mini") + i + len("mini")
			cmd = cmd[:j] + " --standalone" + cmd[j:]
		} else {
			cmd = cmd[:i] + " --standalone" + cmd[i:]
		}
	}
	cfg := map[string]any{}
	if strings.TrimSpace(current) != "" {
		if err := json.Unmarshal([]byte(current), &cfg); err != nil {
			cfg = map[string]any{}
		}
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
