package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"colmeia/daemon/internal/auth"
	"colmeia/daemon/internal/store"
	"colmeia/daemon/internal/term"
)

var lean = os.Getenv("COLMEIA_LEAN") == "1"

// shQuote: aspas simples do shell (bash: '\” ; PowerShell: ”).
func shQuote(s string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// statusCmd: comando da statusline (executado pelo Claude Code, não pelo nosso shell).
func statusCmd(cliDir string) string {
	if runtime.GOOS == "windows" {
		// wrapper .cmd: node do PATH ou o Colmeia.exe (COLMEIA_NODE); cmd/PowerShell/bash entendem aspas duplas
		return `"` + filepath.Join(cliDir, "colmeia-statusline.cmd") + `"`
	}
	f := filepath.Join(cliDir, "colmeia-statusline.mjs")
	return "node " + shQuote(f)
}

// writeClaudeSettings gera (atômico) o settings do Claude Code com a statusline do Colmeia.
func writeClaudeSettings(cliDir string) string {
	file := filepath.Join(auth.ConfigDir(), "claude-settings.json")
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	set := map[string]any{"statusLine": map[string]any{"type": "command", "command": statusCmd(cliDir)}}
	if lean {
		set["permissions"] = map[string]any{"deny": []string{"Artifact", "mcp__claude_ai_Claude_Docs", "ScheduleWakeup", "SendFeedback"}}
	}
	b, _ := jsonIndent(set)
	tmp := file + ".tmp"
	if os.WriteFile(tmp, b, 0o644) != nil || os.Rename(tmp, file) != nil {
		return ""
	}
	return file
}

// withClaudeSettings injeta --settings se o comando começa com `claude`.
func withClaudeSettings(cmd, settings string) string {
	if settings == "" {
		return cmd
	}
	t := strings.TrimLeft(cmd, " \t\r\n")
	if !(t == "claude" || strings.HasPrefix(t, "claude ") || strings.HasPrefix(t, "claude\t") || strings.HasPrefix(t, "claude\n")) {
		return cmd
	}
	if strings.Contains(t, "--settings ") || strings.Contains(t, "--settings=") {
		return cmd
	}
	worker := ""
	if lean && strings.Contains(t, "WORKER.md") {
		worker = " --disallowedTools Agent AskUserQuestion"
	}
	return "claude --settings " + shQuote(settings) + worker + strings.TrimPrefix(t, "claude")
}

type spawnIn struct {
	ID     string `json:"id"`
	Preset string `json:"preset"`
	Title  string `json:"title"`
	Cwd    string `json:"cwd"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
}

// buildSpawn monta o SpawnReq (shell de login, comando do preset, env do card).
func (s *Server) buildSpawn(in spawnIn) term.SpawnReq {
	s.mu.Lock()
	var preset *store.Preset
	for i := range s.presets {
		if s.presets[i].ID == in.Preset {
			preset = &s.presets[i]
		}
	}
	s.mu.Unlock()
	shell, shellArgs := term.DefaultShell()
	cwd := in.Cwd
	home, _ := os.UserHomeDir()
	if cwd == "" {
		cwd = home
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		cwd = home
	}
	args := shellArgs // unix: -l; windows: -NoLogo
	if preset != nil && preset.Command != "" {
		cmd := withClaudeSettings(strings.ReplaceAll(preset.Command, "{docs}", docsArg(s.cfg.DocsDir)), writeClaudeSettings(s.cfg.CliDir))
		if runtime.GOOS == "windows" {
			// PowerShell: -NoExit mantém o shell após o comando
			args = []string{"-NoLogo"}
			if preset.KeepShell {
				args = append(args, "-NoExit")
			}
			args = append(args, "-Command", cmd)
		} else {
			script := cmd
			if preset.KeepShell {
				script = cmd + "; exec " + shell + " -l"
			}
			args = []string{"-l", "-i", "-c", script}
		}
	}
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && !strings.HasPrefix(k, "ELECTRON_") && k != "CHROME_DESKTOP" {
			env[k] = v
		}
	}
	env["TERM"], env["COLORTERM"] = "xterm-256color", "truecolor"
	env["COLMEIA_CARD_ID"], env["COLMEIA_CARD_TITLE"] = in.ID, in.Title
	env["COLMEIA_SOCK"], env["COLMEIA_TOKEN"] = s.cfg.SockPath, auth.TokenFor(in.ID)
	// Windows: a chave é "Path"; sobrescreve a existente (sem duplicar por caixa)
	pathKey := "PATH"
	for k := range env {
		if strings.EqualFold(k, "PATH") {
			pathKey = k
		}
	}
	env[pathKey] = s.cfg.CliDir + string(os.PathListSeparator) + env[pathKey]
	if runtime.GOOS == "windows" {
		// CLIs instaladas pelo npm (opencode, codex, gemini…) são shims .ps1, que o PowerShell recusa com a
		// política padrão (Restricted). Escopo Process = só os processos do card; respeita o que já vier definido.
		hasPolicy := false
		for k := range env {
			if strings.EqualFold(k, "PSExecutionPolicyPreference") {
				hasPolicy = true
			}
		}
		if !hasPolicy {
			env["PSExecutionPolicyPreference"] = "RemoteSigned"
		}
	}
	cols, rows := in.Cols, in.Rows
	if cols == 0 {
		cols = 100
	}
	if rows == 0 {
		rows = 30
	}
	cols, rows = max(20, cols), max(5, rows)
	return term.SpawnReq{ID: in.ID, Preset: in.Preset, Title: in.Title, Cwd: cwd, Command: shell, Args: args, Env: env, Cols: cols, Rows: rows}
}

// docsArg: no Windows (PowerShell) ' vira '' dentro de aspas simples.
func docsArg(d string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(d, "'", "''")
	}
	return d
}
