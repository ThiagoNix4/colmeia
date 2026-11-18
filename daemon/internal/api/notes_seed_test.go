package api

import (
	"os"
	"testing"

	"colmeia/daemon/internal/notes"
	"colmeia/daemon/internal/store"
)

func TestSeedNotesDistinctAndIdempotent(t *testing.T) {
	t.Setenv("COLMEIA_NOTES_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	w := store.Workspace{"name": "seedws"}
	seedNotes(w)
	seedNotes(w) // 2º ws.load não duplica
	ns := notes.List("seedws")
	if len(ns) != 4 {
		t.Fatalf("want 4 notes, got %d", len(ns))
	}
	ids, files, pos := map[string]bool{}, map[string]bool{}, map[[2]float64]bool{}
	for _, n := range ns {
		if ids[n.ID] || files[n.File] || pos[[2]float64{n.X, n.Y}] {
			t.Fatalf("duplicate id/file/position: %+v", n)
		}
		ids[n.ID], files[n.File], pos[[2]float64{n.X, n.Y}] = true, true, true
		b, _ := os.ReadFile(n.File)
		r, _ := notes.ReadNote("seedws", n.ID)
		want := "# " + n.Title + "\n"
		if string(b) != r.Body || len(r.Body) < len(want) || r.Body[:len(want)] != want || countOf(r.Body, "# "+n.Title) != 1 {
			t.Fatalf("bad body for %s: %q", n.Title, r.Body)
		}
	}
}

func countOf(s, sub string) int {
	c := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			c++
		}
	}
	return c
}
