package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"colmeia/daemon/internal/links"
	"colmeia/daemon/internal/notes"
	"colmeia/daemon/internal/store"
	"colmeia/daemon/internal/term"
)

func str(v any, name string) (string, error) {
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("parâmetro obrigatório: %s", name)
	}
	return s, nil
}

func newCardID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type cardInfo struct {
	ID         string `json:"id"`
	Preset     string `json:"preset"`
	Title      string `json:"title"`
	Cwd        string `json:"cwd"`
	Alive      bool   `json:"alive"`
	ExitCode   *int   `json:"exitCode"`
	Pid        *int   `json:"pid"`
	Persistent bool   `json:"persistent"`
	Linked     *bool  `json:"linked,omitempty"`
}

func toCard(i term.Info) cardInfo {
	c := cardInfo{ID: i.ID, Preset: i.Preset, Title: i.Title, Cwd: i.Cwd, Alive: i.Alive, ExitCode: i.ExitCode, Persistent: i.Persistent}
	if i.Pid > 0 {
		p := i.Pid
		c.Pid = &p
	}
	return c
}

// visible: card do workspace ativo (ou ainda sem workspace conhecido). H2.
func (s *Server) visible(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.cardWs[id]
	return !ok || w == s.curWs
}

// visibleInfos: sessões do workspace ativo (all=true: todas).
func (s *Server) visibleInfos(all bool) []term.Info {
	var out []term.Info
	for _, i := range s.cfg.Term.List() {
		if all || s.visible(i.ID) {
			out = append(out, i)
		}
	}
	return out
}

func (s *Server) cards(all bool) []cardInfo {
	out := []cardInfo{}
	for _, i := range s.visibleInfos(all) {
		out = append(out, toCard(i))
	}
	return out
}

func (s *Server) setCardWs(id, ws string) {
	s.mu.Lock()
	s.cardWs[id] = ws
	s.mu.Unlock()
}

// noUI: sem UI aberta, mutações do canvas vão direto ao JSON do workspace (H5).
func (s *Server) noUI() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.uis) == 0
}

func (s *Server) persist(err error) {
	if err != nil {
		log.Printf("persistência do workspace: %v", err)
	}
}

func (s *Server) workspace() string { s.mu.Lock(); defer s.mu.Unlock(); return s.curWs }
func (s *Server) cwd() string       { s.mu.Lock(); defer s.mu.Unlock(); return s.curCwd }

// resolveCard: id exato, título ou prefixo de id (sem diferenciar caixa).
func (s *Server) resolveCard(ref any) (string, error) {
	rs, err := str(ref, "id")
	if err != nil {
		return "", err
	}
	r := strings.ToLower(rs)
	cards := s.visibleInfos(false)
	for _, c := range cards {
		if c.ID == r {
			return c.ID, nil
		}
	}
	pick := func(match func(term.Info) bool) (string, bool, error) {
		var hit []string
		for _, c := range cards {
			if match(c) {
				hit = append(hit, c.ID)
			}
		}
		if len(hit) > 1 {
			return "", false, fmt.Errorf("referência ambígua: %v (%s)", ref, strings.Join(hit, ", "))
		}
		if len(hit) == 1 {
			return hit[0], true, nil
		}
		return "", false, nil
	}
	if id, ok, err := pick(func(c term.Info) bool { return strings.ToLower(c.Title) == r }); err != nil || ok {
		return id, err
	}
	if id, ok, err := pick(func(c term.Info) bool { return strings.HasPrefix(c.ID, r) }); err != nil || ok {
		return id, err
	}
	return "", fmt.Errorf(`card não encontrado: %v. Use "colmeia list".`, ref)
}

// resolveAny: PTY primeiro, depois nota (link/focus).
func (s *Server) resolveAny(ref any) (string, error) {
	if id, err := s.resolveCard(ref); err == nil {
		return id, nil
	}
	r, err := str(ref, "ref")
	if err != nil {
		return "", err
	}
	if n, ok := notes.Get(s.workspace(), r); ok {
		return n.ID, nil
	}
	return "", fmt.Errorf(`card ou nota não encontrado: %v. Use "colmeia list" ou "colmeia note ls".`, ref)
}

// callerWs: o ativo, ou (card sem PTY aqui) o workspace achado em disco.
func (s *Server) callerWs(from string) string {
	if from == "" {
		return s.workspace()
	}
	s.mu.Lock()
	w, ok := s.cardWs[from]
	s.mu.Unlock()
	if ok {
		return w
	}
	if _, ok := s.cfg.Term.Get(from); ok {
		return s.workspace()
	}
	if w := store.WorkspaceOfCard(from); w != "" {
		return w
	}
	return s.workspace()
}

func (s *Server) resolveNote(from string, ref any, check bool) (notes.NoteCard, error) {
	r, err := str(ref, "ref")
	if err != nil {
		return notes.NoteCard{}, err
	}
	n, ok := notes.Get(s.callerWs(from), r)
	if !ok {
		return n, fmt.Errorf(`nota não encontrada: %v. Use "colmeia note ls".`, ref)
	}
	if check {
		if err := s.need(from, n.ID); err != nil {
			return n, err
		}
	}
	return n, nil
}

// activeOnly: mutações do canvas só valem no ws ativo.
func (s *Server) activeOnly(from string) error {
	if from != "" && s.callerWs(from) != s.workspace() {
		return errors.New("seu card é de outro workspace (inativo): troque para ele no Colmeia para alterar o canvas")
	}
	return nil
}

// can: from pode agir sobre id (é ele, controla o card, ou tem aresta com a nota).
func (s *Server) can(from, id string) bool {
	if from == id {
		return true
	}
	ws := s.callerWs(from)
	var diskEdges []store.Edge
	inactive := ws != s.workspace()
	if inactive {
		diskEdges = store.LoadEdges(ws)
	}
	has := func(x, y string) bool {
		if !inactive {
			return links.Controls(x, y)
		}
		for _, e := range diskEdges {
			if e.Source == x && e.Target == y {
				return true
			}
		}
		return false
	}
	if _, isNote := notes.Get(ws, id); isNote {
		return has(from, id) || has(id, from)
	}
	return has(from, id)
}

// need: exige acesso de quem chama ao alvo. Sem from (usuário) = sem restrição.
func (s *Server) need(from, target string) error {
	if from == "" || s.can(from, target) {
		return nil
	}
	if _, ok := s.cfg.Term.Get(from); !ok && store.WorkspaceOfCard(from) == "" {
		return fmt.Errorf(`COLMEIA_CARD_ID desatualizado (%s). Rode "colmeia list", ache seu card e: export COLMEIA_CARD_ID=<seu id>`, from)
	}
	t := target
	if i, ok := s.cfg.Term.Get(target); ok {
		t = i.Title
	} else if n, ok := notes.Get(s.callerWs(from), target); ok {
		t = n.Title
	}
	return fmt.Errorf(`sem link: "%s" não está ligado a você. Ligue no canvas (aresta %s→%s) ou peça ao usuário.`, t, from, target)
}

func (s *Server) makeLink(from, source, target, mode string) error {
	ws := s.workspace()
	_, sn := notes.Get(ws, source)
	_, tn := notes.Get(ws, target)
	if sn && tn {
		return errors.New("nota ↔ nota não pode ser ligada")
	}
	if mode == "pipe" && (sn || tn) {
		return errors.New("pipe só entre terminais")
	}
	if from != "" {
		if err := s.need(from, source); err != nil {
			return err
		}
		if err := s.need(from, target); err != nil {
			return err
		}
	}
	had := links.Controls(source, target)
	links.Add(source, target, mode)
	if s.noUI() {
		s.persist(store.AddEdge(ws, source, target, mode))
	}
	s.emit(Args{"type": "link", "source": source, "target": target, "mode": mode})
	if !had {
		links.NotifyNoteLink(s.cfg.Term, ws, links.Edge{Source: source, Target: target, Mode: mode})
	}
	return nil
}

func numArg(a Args, k string, def float64) float64 {
	if f, ok := a[k].(float64); ok {
		return f
	}
	return def
}

// handleCLI: comandos do CLI (nomes/args/resultados/erros idênticos ao apiServer.ts).
func (s *Server) handleCLI(from, cmd string, a Args) (any, error) {
	t := s.cfg.Term
	ws := s.callerWs(from)
	switch cmd {
	case "ping":
		return Args{"pong": "pong", "version": s.cfg.Version, "cliDir": s.cfg.CliDir, "docsDir": s.cfg.DocsDir}, nil
	case "presets":
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []Args{}
		for _, p := range s.presets {
			out = append(out, Args{"id": p.ID, "label": p.Label, "command": p.Command})
		}
		return out, nil
	case "list":
		cs := s.cards(a["all"] == true)
		if from != "" {
			for i := range cs {
				l := s.can(from, cs[i].ID)
				cs[i].Linked = &l
			}
		}
		return cs, nil
	case "sessions":
		out := []Args{}
		for _, i := range s.visibleInfos(a["all"] == true) {
			out = append(out, Args{"id": i.ID, "title": i.Title, "alive": i.Alive, "persistent": i.Persistent})
		}
		return out, nil
	case "detach":
		id, err := s.resolveCard(a["id"])
		if err != nil {
			return nil, err
		}
		if err := s.need(from, id); err != nil {
			return nil, err
		}
		t.Detach(id)
		s.setCardWs(id, "\x00detached") // L3: some de list/resolve
		if s.noUI() {
			s.persist(store.RemoveCard(s.workspace(), id))
		}
		s.emit(Args{"type": "removeCard", "id": id})
		return Args{"id": id, "detached": true}, nil
	case "spawn":
		if err := s.activeOnly(from); err != nil {
			return nil, err
		}
		presetID, err := str(a["preset"], "preset")
		if err != nil {
			return nil, err
		}
		var label string
		var ids []string
		s.mu.Lock()
		for _, p := range s.presets {
			ids = append(ids, p.ID)
			if p.ID == presetID {
				label = p.Label
			}
		}
		s.mu.Unlock()
		if label == "" {
			return nil, fmt.Errorf("preset desconhecido: %s. Disponíveis: %s", presetID, strings.Join(ids, ", "))
		}
		id := newCardID()
		title, _ := a["title"].(string)
		if title == "" {
			title = fmt.Sprintf("%s %s", label, id[:4])
		}
		cwd, _ := a["cwd"].(string)
		if cwd == "" {
			cwd = s.cwd()
		}
		var near string
		if a["near"] != nil && a["near"] != "" && a["near"] != false {
			if near, err = s.resolveCard(a["near"]); err != nil {
				return nil, err
			}
			if err := s.need(from, near); err != nil {
				return nil, err
			}
		}
		_, info, err := t.Ensure(s.buildSpawn(spawnIn{ID: id, Preset: presetID, Title: title, Cwd: cwd, Cols: 110, Rows: 32}))
		if err != nil {
			return nil, err
		}
		s.setCardWs(id, s.workspace())
		card := Args{"id": id, "preset": presetID, "title": title, "cwd": info.Cwd}
		noUI := s.noUI()
		if noUI {
			s.persist(store.AddCard(s.workspace(), card, near))
		}
		ev := Args{"type": "addCard", "card": card}
		if near != "" {
			ev["near"] = near
		}
		s.emit(ev)
		if near != "" {
			links.Add(near, id, "control")
			if noUI {
				s.persist(store.AddEdge(s.workspace(), near, id, "control"))
			}
			s.emit(Args{"type": "link", "source": near, "target": id, "mode": "control"})
		}
		return toCard(info), nil
	case "send":
		id, err := s.resolveCard(a["id"])
		if err != nil {
			return nil, err
		}
		if err := s.need(from, id); err != nil {
			return nil, err
		}
		text, err := str(a["text"], "text")
		if err != nil {
			return nil, err
		}
		if !t.Send(id, text, a["enter"] != false) {
			return nil, errors.New("card encerrado")
		}
		return Args{"id": id, "sent": true}, nil
	case "key":
		id, err := s.resolveCard(a["id"])
		if err != nil {
			return nil, err
		}
		if err := s.need(from, id); err != nil {
			return nil, err
		}
		data, err := str(a["data"], "data")
		if err != nil {
			return nil, err
		}
		_ = t.Write(id, []byte(data))
		return Args{"id": id}, nil
	case "broadcast":
		text, err := str(a["text"], "text")
		if err != nil {
			return nil, err
		}
		var ids []string
		if arr, ok := a["ids"].([]any); ok && len(arr) > 0 {
			for _, r := range arr {
				id, err := s.resolveCard(r)
				if err != nil {
					return nil, err
				}
				ids = append(ids, id)
			}
		} else {
			for _, i := range s.visibleInfos(false) {
				if from == "" || links.Controls(from, i.ID) {
					ids = append(ids, i.ID)
				}
			}
		}
		if from != "" {
			for _, id := range ids {
				if err := s.need(from, id); err != nil {
					return nil, err
				}
			}
		}
		self, _ := a["exclude"].(string)
		sent := []string{}
		for _, id := range ids {
			if id != self && t.Send(id, text, a["enter"] != false) {
				sent = append(sent, id)
			}
		}
		return Args{"sent": sent}, nil
	case "read":
		id, err := s.resolveCard(a["id"])
		if err != nil {
			return nil, err
		}
		if err := s.need(from, id); err != nil {
			return nil, err
		}
		text, err := t.Read(id, int(numArg(a, "lines", 60)))
		if err != nil {
			return nil, err
		}
		return Args{"id": id, "text": text}, nil
	case "wait":
		id, err := s.resolveCard(a["id"])
		if err != nil {
			return nil, err
		}
		if err := s.need(from, id); err != nil {
			return nil, err
		}
		idle := time.Duration(numArg(a, "idleMs", 5000)) * time.Millisecond
		timeout := time.Duration(numArg(a, "timeoutMs", 10*60_000)) * time.Millisecond
		ok, waited := t.WaitIdle(id, idle, timeout)
		return Args{"id": id, "idle": ok, "waitedMs": waited.Milliseconds()}, nil
	case "link", "unlink":
		if err := s.activeOnly(from); err != nil {
			return nil, err
		}
		source, err := s.resolveAny(a["a"])
		if err != nil {
			return nil, err
		}
		target, err := s.resolveAny(a["b"])
		if err != nil {
			return nil, err
		}
		if cmd == "link" {
			mode := "control"
			if a["mode"] == "pipe" {
				mode = "pipe"
			}
			if err := s.makeLink(from, source, target, mode); err != nil {
				return nil, err
			}
			return Args{"source": source, "target": target, "mode": mode}, nil
		}
		if from != "" {
			if err := s.need(from, source); err != nil {
				return nil, err
			}
			if err := s.need(from, target); err != nil {
				return nil, err
			}
		}
		links.Remove(source, target)
		if s.noUI() {
			s.persist(store.RemoveEdge(s.workspace(), source, target))
		}
		s.emit(Args{"type": "unlink", "source": source, "target": target})
		return Args{"source": source, "target": target}, nil
	case "focus":
		id, err := s.resolveAny(a["id"])
		if err != nil {
			return nil, err
		}
		if err := s.need(from, id); err != nil {
			return nil, err
		}
		s.emit(Args{"type": "focusCard", "id": id})
		return Args{"id": id}, nil
	case "note.list":
		all := notes.List(ws)
		if from == "" {
			return all, nil
		}
		out := []Args{}
		for _, n := range all {
			linked := links.NoteAccess(from, n.ID)
			if linked || a["all"] == true {
				out = append(out, noteMap(n, &linked))
			}
		}
		return out, nil
	case "note.new":
		if err := s.activeOnly(from); err != nil {
			return nil, err
		}
		title, err := str(a["title"], "title")
		if err != nil {
			return nil, err
		}
		near := from
		if a["near"] != nil && a["near"] != "" {
			if near, err = s.resolveCard(a["near"]); err != nil {
				return nil, err
			}
		}
		if near != "" {
			if err := s.need(from, near); err != nil {
				return nil, err
			}
		}
		var body *string
		if b, ok := a["body"].(string); ok {
			body = &b
		}
		note, err := notes.Create(ws, title, body)
		if err != nil {
			return nil, err
		}
		ev := Args{"type": "addNote", "note": note}
		if near != "" {
			ev["near"] = near
		}
		s.emit(ev)
		if near != "" {
			links.Add(near, note.ID, "control")
			if s.noUI() {
				s.persist(store.AddEdge(ws, near, note.ID, "control"))
			}
			s.emit(Args{"type": "link", "source": near, "target": note.ID, "mode": "control"})
		}
		return note, nil
	case "note.read":
		n, err := s.resolveNote(from, a["ref"], true)
		if err != nil {
			return nil, err
		}
		r, err := notes.ReadNote(ws, n.ID)
		if err != nil {
			return nil, err
		}
		return Args{"ref": n.ID, "title": n.Title, "file": n.File, "text": r.Body}, nil
	case "note.write":
		n, err := s.resolveNote(from, a["ref"], true)
		if err != nil {
			return nil, err
		}
		text, err := str(a["text"], "text")
		if err != nil {
			return nil, err
		}
		var mtime float64
		if a["append"] == true {
			mtime, err = notes.Append(ws, n.ID, text)
		} else {
			mtime, err = notes.Write(ws, n.ID, text, nil)
		}
		if err != nil {
			return nil, err
		}
		s.watch.Announce(n.ID, mtime)
		return Args{"ref": n.ID, "file": n.File}, nil
	case "note.path":
		n, err := s.resolveNote(from, a["ref"], true)
		if err != nil {
			return nil, err
		}
		return Args{"ref": n.ID, "file": n.File}, nil
	case "note.rm":
		n, err := s.resolveNote(from, a["ref"], true)
		if err != nil {
			return nil, err
		}
		if err := notes.Remove(ws, n.ID, a["file"] == true); err != nil {
			return nil, err
		}
		if s.noUI() {
			s.persist(store.RemoveEdgesOf(ws, n.ID))
		}
		s.emit(Args{"type": "removeNote", "id": n.ID})
		return Args{"ref": n.ID, "removed": true}, nil
	case "screenshot":
		f, err := str(a["file"], "file")
		if err != nil {
			return nil, err
		}
		file, _ := filepath.Abs(f)
		if from != "" {
			roots := []string{os.TempDir()}
			if i, ok := t.Get(from); ok && i.Cwd != "" {
				roots = append(roots, i.Cwd)
			}
			inside := false
			for _, r := range roots {
				rr, _ := filepath.Abs(r)
				rel, err := filepath.Rel(rr, file)
				if err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
					inside = true
				}
			}
			if !inside {
				return nil, fmt.Errorf("screenshot: caminho fora de %s", strings.Join(roots, " ou "))
			}
		}
		res, err := s.askUI("screenshot", Args{"file": file})
		if err != nil {
			return nil, err
		}
		if p, ok := res.(string); ok && p != "" {
			file = p
		}
		return Args{"file": file}, nil
	case "kill":
		id, err := s.resolveCard(a["id"])
		if err != nil {
			return nil, err
		}
		if err := s.need(from, id); err != nil {
			return nil, err
		}
		if err := t.Kill(id); err != nil {
			return nil, err
		}
		if s.noUI() {
			s.persist(store.RemoveCard(s.workspace(), id))
		}
		s.emit(Args{"type": "removeCard", "id": id})
		return Args{"id": id, "killed": true}, nil
	}
	return nil, fmt.Errorf("comando desconhecido: %s", cmd)
}

func noteMap(n notes.NoteCard, linked *bool) Args {
	m := Args{"id": n.ID, "title": n.Title, "file": n.File, "workspace": n.Workspace, "x": n.X, "y": n.Y, "width": n.Width, "height": n.Height}
	if linked != nil {
		m["linked"] = *linked
	}
	return m
}
