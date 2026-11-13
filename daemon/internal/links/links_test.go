package links

import "testing"

func TestLastResumo(t *testing.T) {
	txt := "blah\n⏺ RESUMO: feito X\n  e mais Y\n\noutro\nRESUMO: eco [from a] x"
	got, ok := LastResumo(txt)
	if !ok || got != "RESUMO: feito X e mais Y" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := LastResumo("nada"); ok {
		t.Fatal("esperava false")
	}
}
