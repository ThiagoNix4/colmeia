package api

import (
	"regexp"
	"strings"
	"time"

	"colmeia/daemon/internal/term"
)

// errMarkers: saída que indica comando que falhou (§0).
var errMarkers = []string{
	"CommandNotFoundException", "ParserError", "ERROR", "Error:", "error:",
	"not recognized", "não é reconhecido",
}

// promptRe: linha de prompt de shell (PowerShell `PS C:\x>`, bash `user@h:~$`, `>`/`%`/`#` soltos).
var promptRe = regexp.MustCompile(`^(PS [^>]*>|\S*[$%#>])\s*$`)

// psPromptRe: prompt do PowerShell com comando digitado (`PS C:\x> cmd`).
var psPromptRe = regexp.MustCompile(`^PS [^>]*>`)

// waitOne espera um card ficar ocioso e classifica: ok | error | exited | timeout.
func waitOne(t term.Manager, id string, idle, timeout time.Duration) Args {
	ok, waited := t.WaitIdle(id, idle, timeout)
	res := Args{"id": id, "title": id, "seconds": int(waited.Seconds() + 0.5)}
	info, found := t.Get(id)
	if found && info.Title != "" {
		res["title"] = info.Title
	}
	text, _ := t.Read(id, 20)
	last, hasErr := scanTail(text)
	switch {
	case !ok:
		res["status"] = "timeout"
	case found && !info.Alive && info.ExitCode != nil:
		res["status"] = "exited"
		res["exitCode"] = *info.ExitCode
	case !found || !info.Alive:
		res["status"] = "exited"
	case hasErr:
		res["status"] = "error"
	default:
		res["status"] = "ok"
	}
	if res["status"] != "ok" && last != "" {
		res["lastLine"] = last
	}
	return res
}

// scanTail: última linha não vazia que não seja prompt + se há marcador de erro nas linhas finais.
func scanTail(text string) (last string, hasErr bool) {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || promptRe.MatchString(l) {
			continue
		}
		if last == "" {
			last = l
		}
	}
	// erro só conta se o card voltou ao prompt (última linha não vazia é prompt)
	end := len(lines) - 1
	for end >= 0 && strings.TrimSpace(lines[end]) == "" {
		end--
	}
	if end < 0 || !promptRe.MatchString(strings.TrimSpace(lines[end])) {
		return last, false
	}
	// só o trecho depois do prompt anterior (comando mais recente)
	for i := end - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if psPromptRe.MatchString(l) || promptRe.MatchString(l) || promptCmdRe.MatchString(l) {
			break
		}
		for _, m := range errMarkers {
			if strings.Contains(l, m) {
				return last, true
			}
		}
	}
	return last, false
}

// promptCmdRe: prompt bash/zsh com o comando digitado (user@h:~/x$ make).
var promptCmdRe = regexp.MustCompile(`^\S+[$%#>]\s+\S`)
