//go:build windows

package api

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// regPath lê um valor Path do registro (REG_SZ ou REG_EXPAND_SZ) já expandido.
func regPath(root registry.Key, sub string) string {
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, typ, err := k.GetStringValue("Path")
	if err != nil {
		return ""
	}
	if typ == registry.EXPAND_SZ {
		if x, err := registry.ExpandString(v); err == nil {
			v = x
		}
	}
	return v
}

// freshPath: PATH atual do sistema + usuário (registro) somado ao herdado, sem duplicatas.
//
// O daemon roda destacado e sobrevive ao app: o PATH que ele herdou congela no momento em que
// subiu. Ferramentas instaladas depois (claude, codex, gemini…) só entram no PATH do registro,
// então cada card novo relê o registro em vez de confiar só no ambiente do daemon.
func freshPath(inherited string) string {
	parts := []string{
		regPath(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`),
		regPath(registry.CURRENT_USER, `Environment`),
	}
	// diretórios de CLIs de agentes que o registro às vezes não lista (npm global, opencode)
	var extra []string
	if ad := os.Getenv("APPDATA"); ad != "" {
		extra = append(extra, filepath.Join(ad, "npm"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		extra = append(extra, filepath.Join(h, ".opencode", "bin"))
	}
	parts = append(parts, strings.Join(extra, string(os.PathListSeparator)), inherited)
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		for _, d := range strings.Split(p, string(os.PathListSeparator)) {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			key := strings.ToLower(filepath.Clean(d))
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, d)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}
