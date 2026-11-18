package links

import "testing"

func TestLastResumo(t *testing.T) {
	txt := "blah\n⏺ RESUMO: feito X\n  e mais Y\n\noutro\nRESUMO: eco [from a] x"
	got, ok := LastResumo(txt)
	if !ok || got != "RESUMO: feito X e mais Y" {
		t.Fatalf("got %q %v", got, ok)
	}
	got, _ = LastResumo("RESUMO: ok Z\nPS C:\\x> ")
	if got != "RESUMO: ok Z" {
		t.Fatalf("prompt: %q", got)
	}
	got, _ = LastResumo("RESUMO: ok W\nuser@h:~$")
	if got != "RESUMO: ok W" {
		t.Fatalf("bash prompt: %q", got)
	}
	for _, tail := range []string{"95%", "PS fix a->b", "C#", "->"} {
		got, _ = LastResumo("RESUMO: x\n" + tail)
		if got != "RESUMO: x "+tail {
			t.Fatalf("keep %q: got %q", tail, got)
		}
	}
	if _, ok := LastResumo("nada"); ok {
		t.Fatal("esperava false")
	}
}
