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

func TestBuiltinOverrideAndReset(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	find := func(l []Preset, id string) Preset {
		for _, p := range l {
			if p.ID == id {
				return p
			}
		}
		t.Fatalf("missing %s", id)
		return Preset{}
	}
	list := LoadPresets()
	if r := find(list, ReviewerID); r.Label != "Revisor" || r.Edited {
		t.Fatalf("default reviewer: %+v", r)
	}
	saved, err := SavePresets([]map[string]any{{"id": ReviewerID, "label": "QA", "color": "#112233", "tool": "claude", "model": "sonnet", "args": "--x"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := find(saved, ReviewerID); r.Label != "QA" || !r.Edited || r.Model != "sonnet" || r.Args != "--x" {
		t.Fatalf("override: %+v", r)
	}
	if m := find(LoadPresets(), ReviewerID); m.Label != "QA" {
		t.Fatalf("override not persisted: %+v", m)
	}
	cur, id, err := ResetBuiltinPreset(LoadPresets(), "QA")
	if err != nil || id != ReviewerID {
		t.Fatal(id, err)
	}
	if r := find(cur, ReviewerID); r.Label != "Revisor" || r.Edited || r.Args != "" {
		t.Fatalf("reset: %+v", r)
	}
	// rótulo "Revisor" de usuário → padrão vira "Revisor (padrão)" e não conta como editado
	cur, _, err = AddUserPreset(LoadPresets(), "Revisor", "claude", "haiku", false)
	if err == nil {
		t.Fatalf("expected duplicate label error, got %+v", cur)
	}
}

func TestCardModelPersisted(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	ws := Workspace{"name": "m", "cwd": "/x", "edges": []any{}, "cards": []any{
		map[string]any{"id": "c1", "preset": "claude", "model": " haiku "},
		map[string]any{"id": "c2", "preset": "claude"}, // workspace antigo, sem o campo
	}}
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	if got := CardModel("c1"); got != "haiku" {
		t.Fatalf("c1=%q", got)
	}
	if got := CardModel("c2"); got != "" {
		t.Fatalf("c2=%q", got)
	}
	if got := CardModel("nope"); got != "" {
		t.Fatalf("nope=%q", got)
	}
}
