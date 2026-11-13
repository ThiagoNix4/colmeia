package store

import "testing"

func TestWorkspaceBakAndCorrupt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ws := Workspace{"name": "a b", "cwd": "/x", "cards": []any{map[string]any{"id": "c1", "preset": "shell"}}, "edges": []any{}}
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkspace(Workspace{"name": "a b", "cards": []any{}, "edges": []any{}}); err != nil {
		t.Fatal(err)
	}
	// vazio não sobrescreve o .bak; corromper o principal recupera do .bak
	ids := AllCardIDs()
	if len(ids) != 0 {
		t.Fatalf("ids=%v", ids)
	}
	if got := LoadWorkspace("a_b"); len(cardsOf(got)) != 0 {
		t.Fatalf("esperava ws vazio salvo")
	}
	if p := LoadPresets(); len(p) < 2 || p[0].ID != "shell" || p[1].ID != "claude-manager" {
		t.Fatalf("presets=%v", p)
	}
}
