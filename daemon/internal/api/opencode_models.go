package api

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"colmeia/daemon/internal/store"
)

var (
	ocModelsMu   sync.Mutex
	ocModelsAt   time.Time
	ocModelsList []string
)

// opencodeModels: ids provider/modelo de "opencode models" (cache de 60 s; vazio se não houver opencode).
func opencodeModels() []string {
	ocModelsMu.Lock()
	defer ocModelsMu.Unlock()
	if ocModelsList != nil && time.Since(ocModelsAt) < time.Minute {
		return ocModelsList
	}
	out := []string{}
	if bin, err := exec.LookPath("opencode"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, bin, "models")
		hideWindow(c)
		b, _ := c.Output()
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); strings.Contains(l, "/") && !strings.ContainsAny(l, " \t") {
				out = append(out, l)
			}
		}
	}
	ocModelsList, ocModelsAt = out, time.Now()
	return out
}

// ocCmdModelRe: -m/--model só logo após "opencode" ou "opencode mini".
var ocCmdModelRe = regexp.MustCompile(`^\s*opencode(?:\s+mini)?\s+(?:-m|--model)(?:\s+|=)(\S+)`)

// checkPresetModel: recusa abrir card opencode com modelo fora de provider/modelo (override ou do preset).
func (s *Server) checkPresetModel(presetID, override string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.presets {
		if p.ID != presetID {
			continue
		}
		if !isOpencodeCmd(p.Command) && p.Tool != "opencode" {
			return nil
		}
		m := strings.TrimSpace(override)
		if m == "" {
			m = p.Model
		}
		if m == "" {
			if f := ocCmdModelRe.FindStringSubmatch(p.Command); f != nil {
				m = f[1]
			}
		}
		return store.ValidOpencodeModel(m)
	}
	return nil
}
