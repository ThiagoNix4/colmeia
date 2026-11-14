package api

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
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

// opencodeHelp: saída de "opencode --help" (~0,3 s); vazio se não achar ou travar.
func opencodeHelp() string {
	bin, err := exec.LookPath("opencode")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, bin, "--help").CombinedOutput()
	return string(out)
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
