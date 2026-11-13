package api

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"sync"

	"colmeia/daemon/internal/auth"
	"colmeia/daemon/internal/links"
	"colmeia/daemon/internal/notes"
	"colmeia/daemon/internal/store"
	"colmeia/daemon/internal/usage"
)

// handle: ui.* só para conexões anexadas; o resto passa por caller + comandos do CLI.
func (s *Server) handle(c *conn, cmd string, a Args) (any, error) {
	switch {
	case cmd == "ui.attach":
		return s.attach(c, a)
	case cmd == "ui.reply":
		if !c.ui.Load() {
			return nil, errors.New("ui.reply: conexão não anexada")
		}
		return nil, s.uiReply(c, a)
	case strings.Contains(cmd, ".") && c.ui.Load() && s.isUICmd(cmd):
		return s.handleUI(cmd, a)
	}
	from, err := s.caller(c, a)
	if err != nil {
		return nil, err
	}
	return s.handleCLI(from, cmd, a)
}

func (s *Server) isUICmd(cmd string) bool {
	switch cmd {
	case "note.list", "note.new", "note.read", "note.write", "note.path", "note.rm":
		return false
	}
	return true
}

func (s *Server) attach(c *conn, a Args) (any, error) {
	// M1: Electron iniciado de dentro de um card (dev) é descendente do daemon; o segredo vale como prova.
	// Só rejeita órfão sem card? Também aceita: quem tem o segredo (mesmo uid) já controla tudo.
	sec, _ := a["secret"].(string)
	if sec == "" || !auth.CheckSecret(sec) {
		return nil, errors.New("ui.attach: segredo inválido")
	}
	s.mu.Lock()
	if !c.ui.Load() {
		c.ui.Store(true)
		s.uis = append(s.uis, c)
	}
	s.mu.Unlock()
	log.Printf("ui.attach (origem=%d)", c.origin)
	go s.prune() // M8: sessões fora de qualquer workspace morrem (após responder)
	h, _ := os.UserHomeDir()
	return Args{"socketPath": s.cfg.SockPath, "presetsPath": store.PresetsPath(), "home": h, "cliDir": s.cfg.CliDir, "docsDir": s.cfg.DocsDir}, nil
}

func (s *Server) uiReply(c *conn, a Args) error {
	rid, _ := a["rid"].(string)
	s.mu.Lock()
	p := s.pendingUI[rid]
	delete(s.pendingUI, rid)
	s.mu.Unlock()
	if p == nil {
		return errors.New("ui.reply: rid desconhecido")
	}
	r := reply{ok: a["ok"] == true, result: a["result"]}
	if e, ok := a["error"].(string); ok {
		r.err = e
	}
	if !r.ok && r.err == "" {
		r.err = "falha na UI"
	}
	p.ch <- r
	return nil
}

func decode(v any, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (s *Server) handleUI(cmd string, a Args) (any, error) {
	t := s.cfg.Term
	id, _ := a["id"].(string)
	ws, _ := a["ws"].(string)
	switch cmd {
	case "pty.ensure":
		var in spawnIn
		if err := decode(a["req"], &in); err != nil || in.ID == "" {
			return nil, errors.New("parâmetro obrigatório: req")
		}
		replay, info, err := t.Ensure(s.buildSpawn(in))
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		if _, ok := s.cardWs[in.ID]; !ok {
			s.cardWs[in.ID] = s.curWs
		}
		s.mu.Unlock()
		return Args{"replay": replay, "offset": info.Offset, "info": toCard(info)}, nil
	case "pty.restart":
		return nil, t.Restart(id)
	case "pty.write":
		data, _ := a["data"].(string)
		_ = t.Write(id, []byte(data))
		return nil, nil
	case "pty.resize":
		t.Resize(id, int(numArg(a, "cols", 0)), int(numArg(a, "rows", 0)))
		return nil, nil
	case "pty.kill":
		_ = t.Kill(id)
		return nil, nil
	case "pty.detachAll":
		for _, i := range t.List() {
			t.Detach(i.ID)
		}
		return nil, nil
	case "pty.killAll":
		var wg sync.WaitGroup
		ids, _ := a["\x00ids"].([]string)
		for _, id := range ids {
			wg.Add(1)
			go func(id string) { defer wg.Done(); _ = t.Kill(id) }(id)
		}
		wg.Wait()
		return nil, nil
	case "pty.setMeta":
		var p struct {
			Title *string `json:"title"`
		}
		_ = decode(a["patch"], &p)
		if p.Title != nil {
			t.SetMeta(id, *p.Title)
		}
		return nil, nil
	case "pty.send":
		text, _ := a["text"].(string)
		out := []string{}
		if arr, ok := a["ids"].([]any); ok {
			for _, v := range arr {
				if x, ok := v.(string); ok && t.Send(x, text, true) {
					out = append(out, x)
				}
			}
		}
		return out, nil
	case "pty.list":
		return s.cards(false), nil
	case "app.newId":
		return newCardID(), nil
	case "app.flush":
		t.FlushAll()
		return nil, nil
	case "presets.get":
		p := store.LoadPresets()
		s.mu.Lock()
		s.presets = p
		s.mu.Unlock()
		return p, nil
	case "presets.save":
		var list []map[string]any
		if err := decode(a["list"], &list); err != nil || list == nil {
			return nil, errors.New("lista de agentes inválida")
		}
		p, err := store.SavePresets(list)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.presets = p
		s.mu.Unlock()
		return p, nil
	case "ws.list":
		return store.ListWorkspaces(), nil
	case "ws.load":
		name, _ := a["name"].(string)
		w := store.LoadWorkspace(name)
		s.mu.Lock()
		s.curCwd, s.curWs = store.WorkspaceCwd(w), store.WorkspaceName(w)
		cur := s.curWs
		s.mu.Unlock()
		s.syncCards(cur, w)
		links.SetEdges(links.FromStore(store.EdgesOf(w)))
		s.watch.Watch(cur)
		return w, nil
	case "ws.save":
		w, ok := a["ws"].(map[string]any)
		if !ok {
			return nil, errors.New("parâmetro obrigatório: ws")
		}
		s.mu.Lock()
		s.curCwd = store.WorkspaceCwd(w)
		s.curWs = store.WorkspaceName(w)
		cur := s.curWs
		s.mu.Unlock()
		s.syncCards(cur, w)
		if err := store.SaveWorkspace(w); err != nil {
			return nil, err
		}
		s.watch.Watch(cur)
		return nil, nil
	case "ws.delete":
		name, _ := a["name"].(string)
		store.DeleteWorkspace(name)
		notes.DeleteWorkspace(name)
		return nil, nil
	case "ws.rename":
		from, _ := a["from"].(string)
		to, _ := a["to"].(string)
		name, err := store.RenameWorkspace(from, to)
		if err != nil {
			return nil, err
		}
		notes.RenameWorkspace(from, name)
		s.mu.Lock()
		renamed := s.curWs == from
		if renamed {
			s.curWs = name
		}
		for id, w := range s.cardWs { // L2
			if w == from {
				s.cardWs[id] = name
			}
		}
		s.mu.Unlock()
		if renamed {
			s.watch.Watch(name)
		}
		return name, nil
	case "links.set":
		var list []links.Edge
		if raw, ok := a["edges"].([]any); ok {
			_ = decode(raw, &list)
		}
		s.edges.Set(s.cfg.Term, s.workspace(), list)
		return nil, nil
	case "notes.list":
		return notes.List(ws), nil
	case "notes.create":
		title, _ := a["title"].(string)
		return notes.Create(ws, title, nil)
	case "notes.update":
		patch, _ := a["patch"].(map[string]any)
		return notes.Update(ws, id, patch)
	case "notes.remove":
		return nil, notes.Remove(ws, id, a["deleteFile"] == true)
	case "notes.read":
		ref, _ := a["ref"].(string)
		r, err := notes.ReadNote(ws, ref)
		if err != nil {
			return nil, err
		}
		if n, ok := notes.Get(ws, ref); ok {
			s.watch.MarkSeen(n.ID, r.Mtime)
		}
		return r, nil
	case "notes.write":
		ref, _ := a["ref"].(string)
		text, _ := a["text"].(string)
		var base *float64
		if f, ok := a["baseMtime"].(float64); ok {
			base = &f
		}
		m, err := notes.Write(ws, ref, text, base)
		if err != nil {
			return nil, err
		}
		if n, ok := notes.Get(ws, ref); ok {
			s.watch.MarkSeen(n.ID, m) // escrita do próprio renderer: sem note-changed
		}
		return Args{"mtime": m}, nil
	case "notes.root":
		return notes.Root(), nil
	case "usage.get":
		return usage.GetReport(), nil
	case "usage.setSettings":
		st, _ := a["s"].(map[string]any)
		return usage.SetSettings(st), nil
	}
	return nil, errors.New("comando desconhecido: " + cmd)
}

// syncCards: cards do workspace ws passam a pertencer a ele (H2).
func (s *Server) syncCards(ws string, w map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, n := range s.cardWs {
		if n == ws {
			delete(s.cardWs, id)
		}
	}
	for _, id := range store.CardIDsOf(w) {
		s.cardWs[id] = ws
	}
}

// prune: M8 — mata/limpa sessões que não estão em nenhum workspace.
func (s *Server) prune() { s.cfg.Term.Prune(store.AllCardIDs()) }
