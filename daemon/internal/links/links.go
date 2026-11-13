// Package links: arestas do canvas (direcionadas, em memória) + efeitos (nota ligada, pipe de RESUMO).
package links

import (
	"crypto/sha1"
	"encoding/hex"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"colmeia/daemon/internal/notes"
	"colmeia/daemon/internal/store"
	"colmeia/daemon/internal/term"
)

type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Mode   string `json:"mode"`
}

var (
	mu    sync.Mutex
	edges = map[string]Edge{}
)

func key(a, b string) string { return a + "|" + b }

func normMode(m string) string {
	if m == "pipe" {
		return "pipe"
	}
	return "control"
}

func SetEdges(list []Edge) {
	mu.Lock()
	defer mu.Unlock()
	edges = map[string]Edge{}
	for _, e := range list {
		if e.Source != "" && e.Target != "" {
			edges[key(e.Source, e.Target)] = Edge{e.Source, e.Target, normMode(e.Mode)}
		}
	}
}

// FromStore converte as arestas lidas do workspace.
func FromStore(es []store.Edge) []Edge {
	out := make([]Edge, 0, len(es))
	for _, e := range es {
		out = append(out, Edge{e.Source, e.Target, normMode(e.Mode)})
	}
	return out
}

func Get() []Edge {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Edge, 0, len(edges))
	for _, e := range edges {
		out = append(out, e)
	}
	return out
}

func Add(source, target, mode string) {
	mu.Lock()
	edges[key(source, target)] = Edge{source, target, normMode(mode)}
	mu.Unlock()
}

// Remove apaga a aresta nos dois sentidos.
func Remove(a, b string) {
	mu.Lock()
	delete(edges, key(a, b))
	delete(edges, key(b, a))
	mu.Unlock()
}

// Controls: a pode comandar b (aresta a→b, qualquer modo).
func Controls(a, b string) bool {
	mu.Lock()
	defer mu.Unlock()
	_, ok := edges[key(a, b)]
	return ok
}

// NoteAccess: card ↔ nota, aresta em qualquer direção.
func NoteAccess(card, note string) bool {
	return Controls(card, note) || Controls(note, card)
}

// Pipes: destinos de pipe saindo de a.
func Pipes(a string) []string {
	mu.Lock()
	defer mu.Unlock()
	var out []string
	for _, e := range edges {
		if e.Source == a && e.Mode == "pipe" {
			out = append(out, e.Target)
		}
	}
	sort.Strings(out)
	return out
}

// ---- linkEffects ----

// NotifyNoteLink: ao criar aresta card↔nota, avisa o card (só terminal com agente, não shell).
func NotifyNoteLink(m term.Manager, workspace string, e Edge) {
	aNote, aOK := notes.Get(workspace, e.Source)
	bNote, bOK := notes.Get(workspace, e.Target)
	if aOK == bOK {
		return // nenhum ou nota↔nota
	}
	note, cardID := bNote, e.Source
	if aOK {
		note, cardID = aNote, e.Target
	}
	s, ok := m.Get(cardID)
	if !ok || !s.Alive || s.Preset == "shell" {
		return
	}
	m.Send(cardID, `Context note linked: "`+note.Title+`". Read: colmeia note read "`+note.Title+`"`, true)
}

// EdgeTracker: avisa nota só para ligação nova de fato (par sem direção, janela de 10s).
type EdgeTracker struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

const renotify = 10 * time.Second

func NewEdgeTracker() *EdgeTracker { return &EdgeTracker{seen: map[string]time.Time{}} }

func pairKey(e Edge) string {
	p := []string{e.Source, e.Target}
	sort.Strings(p)
	return strings.Join(p, "|")
}

// Set substitui as arestas (links.set) e notifica as novas.
func (t *EdgeTracker) Set(m term.Manager, workspace string, list []Edge) {
	now := time.Now()
	t.mu.Lock()
	for _, e := range Get() {
		t.seen[pairKey(e)] = now
	}
	SetEdges(list)
	var fresh []Edge
	for _, e := range Get() {
		k := pairKey(e)
		seen, ok := t.seen[k]
		t.seen[k] = now
		if !ok || now.Sub(seen) > renotify {
			fresh = append(fresh, e)
		}
	}
	t.mu.Unlock()
	for _, e := range fresh {
		NotifyNoteLink(m, workspace, e)
	}
}

var (
	resumoLine = regexp.MustCompile(`^[\s\x{00A0}\x{FEFF}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}⏺•●*>\-│]*RESUMO:`)
	lead       = regexp.MustCompile(`^[\s\x{00A0}\x{FEFF}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}⏺•●*>│]+`)
	dash       = regexp.MustCompile(`^-\s*`)
)

// LastResumo: último bloco RESUMO (até ~5 linhas) numa linha só.
func LastResumo(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	i := len(lines) - 1
	// linhas com "[from X]" são eco de pipe: nunca reencaminhar (evita laço A→B→A)
	for i >= 0 && (!resumoLine.MatchString(lines[i]) || strings.Contains(lines[i], "[from ")) {
		i--
	}
	if i < 0 {
		return "", false
	}
	var block []string
	for j := i; j < len(lines) && len(block) < 5; j++ {
		l := strings.TrimSpace(lead.ReplaceAllString(lines[j], ""))
		if j > i && (l == "" || strings.Contains(l, "[from ")) {
			break
		}
		if l != "" {
			block = append(block, l)
		}
	}
	return dash.ReplaceAllString(strings.Join(block, " "), ""), true
}

// Piper encaminha o RESUMO de A para destinos pipe quando A fica ocioso (3s).
type Piper struct {
	m         term.Manager
	mu        sync.Mutex
	timers    map[string]*time.Timer
	forwarded map[string]string
}

const idleMs = 3 * time.Second

func NewPiper(m term.Manager) *Piper {
	return &Piper{m: m, timers: map[string]*time.Timer{}, forwarded: map[string]string{}}
}

// OnData: chamar a cada saída de PTY.
func (p *Piper) OnData(id string) {
	if len(Pipes(id)) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if t := p.timers[id]; t != nil {
		t.Stop()
	}
	p.timers[id] = time.AfterFunc(idleMs, func() {
		defer func() { // M9
			if r := recover(); r != nil {
				log.Printf("panic no piper: %v", r)
			}
		}()
		p.mu.Lock()
		delete(p.timers, id)
		p.mu.Unlock()
		p.onIdle(id)
	})
}

func (p *Piper) onIdle(id string) {
	targets := Pipes(id)
	if len(targets) == 0 {
		return
	}
	text, err := p.m.Read(id, 200)
	if err != nil {
		return
	}
	resumo, ok := LastResumo(text)
	if !ok {
		return
	}
	sum := sha1.Sum([]byte(resumo))
	hash := hex.EncodeToString(sum[:])
	title := id
	if i, ok := p.m.Get(id); ok {
		title = i.Title
	}
	for _, t := range targets {
		k := id + "|" + t
		p.mu.Lock()
		same := p.forwarded[k] == hash
		p.mu.Unlock()
		if same {
			continue
		}
		if p.m.Send(t, "[from "+title+"] "+resumo, true) {
			p.mu.Lock()
			p.forwarded[k] = hash
			p.mu.Unlock()
		}
	}
}
