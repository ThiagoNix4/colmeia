//go:build windows

package api

import (
	"os"
	"testing"
)

// Shims .ps1 do npm (opencode etc.) exigem política de execução liberada no card;
// o que o usuário já definiu no ambiente é respeitado.
func TestBuildSpawnExecutionPolicy(t *testing.T) {
	const key = "PSExecutionPolicyPreference"
	s := &Server{}
	policy := func() string { return s.buildSpawn(spawnIn{ID: "c", Cwd: t.TempDir()}).Env[key] }

	t.Setenv(key, "AllSigned")
	if got := policy(); got != "AllSigned" {
		t.Fatalf("valor do usuário sobrescrito: %q", got)
	}
	if err := os.Unsetenv(key); err != nil { // t.Setenv restaura o valor original no fim
		t.Fatal(err)
	}
	if got := policy(); got != "RemoteSigned" {
		t.Fatalf("padrão = %q, esperado RemoteSigned", got)
	}
}
