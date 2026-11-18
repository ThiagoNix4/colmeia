package api

import "testing"

func TestLayoutNoOverlap(t *testing.T) {
	items := []layItem{{"m", "claude-manager", "G"}, {"a", "x", "A"}, {"b", "x", "B"}, {"c", "x", "C"}, {"d", "x", "D"}}
	got := layoutCards(tidyOrder(items))
	if r := got["m"]; r.X != 0 || r.Y != 0 {
		t.Fatalf("manager at %v", r)
	}
	for i, a := range items {
		for _, b := range items[i+1:] {
			ra, rb := got[a.ID], got[b.ID]
			if ra.X < rb.X+rb.W && rb.X < ra.X+ra.W && ra.Y < rb.Y+rb.H && rb.Y < ra.Y+ra.H {
				t.Fatalf("overlap %s %s", a.ID, b.ID)
			}
		}
	}
	if n := noteSlot(0); n.X+n.W > 0 {
		t.Fatalf("note column overlaps manager: %v", n)
	}
}

func TestRecentSlotsBusy(t *testing.T) {
	noteRecentSlot("r1", rect{layCardW + 2*layGap, 0, layCardW, layCardH})
	if got := recentSlotsExcept("r2"); len(got) == 0 {
		t.Fatal("recent slot missing")
	}
	if got := recentSlotsExcept("r1"); len(got) != 0 {
		t.Fatalf("own slot must be excluded: %v", got)
	}
}
