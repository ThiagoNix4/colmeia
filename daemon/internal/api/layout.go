package api

import (
	"sort"
	"sync"
	"time"

	"colmeia/daemon/internal/notes"
	"colmeia/daemon/internal/store"
)

// Grade do canvas: cards uniformes, Gerente em (0,0), operárias em grade à direita, notas em coluna à esquerda.
const (
	layCardW, layCardH = 640.0, 400.0
	layNoteW, layNoteH = 420.0, 320.0
	layGap             = 40.0
	layCols            = 3
)

type layItem struct{ ID, Preset, Title string }

type rect = struct{ X, Y, W, H float64 }

func rectArgs(id string, r rect) Args {
	return Args{"id": id, "x": r.X, "y": r.Y, "width": r.W, "height": r.H}
}

// layoutCards: Gerentes empilhados em x=0; demais em grade (layCols colunas) à direita, na ordem recebida.
func layoutCards(items []layItem) map[string]rect {
	out := map[string]rect{}
	mi, wi := 0, 0
	for _, it := range items {
		if it.Preset == store.ManagerID {
			out[it.ID] = rect{0, float64(mi) * (layCardH + layGap), layCardW, layCardH}
			mi++
			continue
		}
		col, row := wi%layCols, wi/layCols
		out[it.ID] = rect{layCardW + 2*layGap + float64(col)*(layCardW+layGap), float64(row) * (layCardH + layGap), layCardW, layCardH}
		wi++
	}
	return out
}

// noteSlot: i-ésima nota, coluna própria à esquerda do Gerente.
func noteSlot(i int) rect {
	return rect{-(layNoteW + 2*layGap), float64(i) * (layNoteH + layGap), layNoteW, layNoteH}
}

// tidyOrder: ordem do `tidy`: Gerente(s), operárias agrupadas por preset (Revisor e terminais por último), título dentro do grupo.
func tidyOrder(items []layItem) []layItem {
	rank := func(p string) int {
		switch p {
		case store.ManagerID:
			return 0
		case store.ReviewerID:
			return 2
		case "shell":
			return 3
		}
		return 1
	}
	out := append([]layItem{}, items...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rank(a.Preset), rank(b.Preset); ra != rb {
			return ra < rb
		}
		if a.Preset != b.Preset {
			return a.Preset < b.Preset
		}
		return a.Title < b.Title
	})
	return out
}

// cardItems: cards visíveis do workspace ativo na ordem de criação.
func (s *Server) cardItems() []layItem {
	infos := s.visibleInfos(false)
	sort.SliceStable(infos, func(i, j int) bool { return infos[i].Created.Before(infos[j].Created) })
	out := make([]layItem, 0, len(infos))
	for _, i := range infos {
		out = append(out, layItem{i.ID, i.Preset, i.Title})
	}
	return out
}

var (
	recentMu    sync.Mutex
	recentSlots = map[string]recentSlot{}
)

type recentSlot struct {
	r  rect
	at time.Time
}

const recentTTL = 10 * time.Second

// noteRecentSlot: guarda slots dados há pouco (a UI ainda não salvou o card no disco).
func noteRecentSlot(id string, r rect) {
	recentMu.Lock()
	defer recentMu.Unlock()
	recentSlots[id] = recentSlot{r, time.Now()}
}

func recentSlotsExcept(id string) []rect {
	recentMu.Lock()
	defer recentMu.Unlock()
	var out []rect
	for k, v := range recentSlots {
		if time.Since(v.at) > recentTTL {
			delete(recentSlots, k)
		} else if k != id {
			out = append(out, v.r)
		}
	}
	return out
}

// cardSlot: primeiro slot da grade livre (sem UI aberta confere os cards do disco; com UI a UI valida colisão).
func (s *Server) cardSlot(id string) (rect, bool) {
	var preset string
	for _, it := range s.cardItems() {
		if it.ID == id {
			preset = it.Preset
		}
	}
	if preset == "" {
		return rect{}, false
	}
	busy := store.CardRects(s.workspace(), id) // a UI autosalva o workspace em ~1s
	busy = append(busy, recentSlotsExcept(id)...)
	free := func(r rect) bool {
		for _, b := range busy {
			if r.X < b.X+b.W+layGap && b.X < r.X+r.W+layGap && r.Y < b.Y+b.H+layGap && b.Y < r.Y+r.H+layGap {
				return false
			}
		}
		return true
	}
	for i := 0; i < 500; i++ {
		var r rect
		if preset == store.ManagerID {
			r = rect{0, float64(i) * (layCardH + layGap), layCardW, layCardH}
		} else {
			r = rect{layCardW + 2*layGap + float64(i%layCols)*(layCardW+layGap), float64(i/layCols) * (layCardH + layGap), layCardW, layCardH}
		}
		if free(r) {
			noteRecentSlot(id, r)
			return r, true
		}
	}
	return rect{}, false
}

// tidy: reorganiza cards + notas do workspace ativo e avisa a UI (evento "layout").
func (s *Server) tidy() Args {
	ws := s.workspace()
	cr := layoutCards(tidyOrder(s.cardItems()))
	cardsEv := []Args{}
	for id, r := range cr {
		cardsEv = append(cardsEv, rectArgs(id, r))
	}
	sort.Slice(cardsEv, func(i, j int) bool { return cardsEv[i]["id"].(string) < cardsEv[j]["id"].(string) })
	notesEv := []Args{}
	ns := notes.List(ws)
	sort.SliceStable(ns, func(i, j int) bool { return ns[i].Title < ns[j].Title })
	for i, n := range ns {
		r := noteSlot(i)
		_, _ = notes.Update(ws, n.ID, map[string]any{"x": r.X, "y": r.Y, "width": r.W, "height": r.H})
		notesEv = append(notesEv, rectArgs(n.ID, r))
	}
	if s.noUI() {
		s.persist(store.SetCardRects(ws, cr))
	}
	s.emit(Args{"type": "layout", "cards": cardsEv, "notes": notesEv})
	return Args{"cards": len(cardsEv), "notes": len(notesEv)}
}
