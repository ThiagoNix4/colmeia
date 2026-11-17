//go:build windows

package api

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// freshPath precisa trazer o PATH do usuário do registro mesmo quando o herdado não o tem
// (daemon iniciado antes de instalar uma CLI) e não pode duplicar entradas.
func TestFreshPathIncludesRegistryUserPath(t *testing.T) {
	user := regPath(registry.CURRENT_USER, `Environment`)
	got := freshPath(`C:\herdado`)
	if !strings.HasSuffix(got, `;C:\herdado`) && got != `C:\herdado` {
		t.Fatalf("PATH herdado deveria vir por último: %q", got)
	}
	for _, d := range strings.Split(user, ";") {
		if d = strings.TrimSpace(d); d != "" && !strings.Contains(strings.ToLower(got), strings.ToLower(d)) {
			t.Fatalf("faltou %q do PATH do usuário em %q", d, got)
		}
	}
	twice := freshPath(got)
	if len(strings.Split(twice, ";")) != len(strings.Split(got, ";")) {
		t.Fatalf("entradas duplicadas: %q", twice)
	}
}
