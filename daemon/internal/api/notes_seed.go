package api

import (
	"sync"

	"colmeia/daemon/internal/notes"
	"colmeia/daemon/internal/store"
)

var seedMu sync.Mutex

// seedNotes: workspace novo (sem cards, arestas nem notas) ganha notas-modelo curtas, em inglês.
func seedNotes(w store.Workspace) {
	seedMu.Lock() // ws.load concorrente não semeia duas vezes
	defer seedMu.Unlock()
	ws := store.WorkspaceName(w)
	if ws == "" || len(notes.List(ws)) > 0 {
		return
	}
	if cs, _ := w["cards"].([]any); len(cs) > 0 {
		return
	}
	if es, _ := w["edges"].([]any); len(es) > 0 {
		return
	}
	for i, t := range []struct{ title, body string }{
		{"Contexto do Workspace", "# Contexto do Workspace\n\ngoal: \nstack: \nrepo: \nconstraints: \nstatus: \n"},
		{"Comandos", "# Comandos\n\nbuild: \ntest: \nrun: \nlint: \n"},
		{"Decisoes", "# Decisoes\n\n<date> | decision: | why: \n"},
		{"Handoff", "# Handoff\n\n<card>: done X (file:line) → state ✓/✗ → next Y\n"},
	} {
		body := t.body
		// posições distintas (coluna de notas): em (0,0) as 4 notas empilhavam
		if n, err := notes.Create(ws, t.title, &body); err == nil {
			r := noteSlot(i)
			_, _ = notes.Update(ws, n.ID, map[string]any{"x": r.X, "y": r.Y, "width": r.W, "height": r.H})
		}
	}
}
