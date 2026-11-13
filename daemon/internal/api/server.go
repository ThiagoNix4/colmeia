// Package api: servidor de socket (Contract B) — comandos do CLI, comandos ui.* e eventos.
package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"colmeia/daemon/internal/auth"
	"colmeia/daemon/internal/links"
	"colmeia/daemon/internal/notes"
	"colmeia/daemon/internal/store"
	"colmeia/daemon/internal/term"
	"colmeia/daemon/internal/usage"
)

type Args = map[string]any

type Config struct {
	Term      term.Manager
	SockPath  string
	DocsDir   string
	CliDir    string
	DaemonPid int
	Version   string // hash do binário (ping)
}

type Server struct {
	cfg    Config
	ln     net.Listener
	piper  *links.Piper
	edges  *links.EdgeTracker
	watch  *notes.Watcher
	stopUs func()

	mu        sync.Mutex // estado: ws ativo, presets, UIs, pendentes
	curWs     string
	curCwd    string
	presets   []store.Preset
	uis       []*conn
	pendingUI map[string]*pendingReq
	cardWs    map[string]string // card → workspace (H2)
	ridSeq    atomic.Uint64

	sendMu  sync.Mutex // serializa extração+envio de pty.data/exit (ordem)
	pmu     sync.Mutex // dados de PTY pendentes p/ a UI
	pending map[string]*chunk
	flushT  *time.Timer
	cancel  func()
	done    chan struct{}
}

type pendingReq struct {
	c  *conn
	ch chan reply
}

type reply struct {
	ok     bool
	result any
	err    string
}

type conn struct {
	c      net.Conn
	out    chan []byte
	done   chan struct{}
	once   sync.Once
	ui     atomic.Bool
	origin auth.Origin
	cardID string
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, pendingUI: map[string]*pendingReq{}, pending: map[string]*chunk{}, cardWs: store.CardWorkspaces(), done: make(chan struct{})}
	s.presets = store.LoadPresets()
	ws := store.LoadWorkspace("")
	s.curWs, s.curCwd = store.WorkspaceName(ws), store.WorkspaceCwd(ws)
	links.SetEdges(links.FromStore(store.EdgesOf(ws)))
	s.piper = links.NewPiper(cfg.Term)
	s.edges = links.NewEdgeTracker()
	s.watch = notes.NewWatcher(func(id string, mtime float64) {
		s.emit(Args{"type": "note-changed", "id": id, "mtime": mtime})
	})
	return s
}

// Start abre o socket, reconcilia históricos e liga os eventos.
func (s *Server) Start() error {
	ln, err := listenSock(s.cfg.SockPath)
	if err != nil {
		return err
	}
	s.ln = ln
	s.cfg.Term.Prune(store.AllCardIDs())
	s.watch.Watch(s.curWs)
	s.stopUs = usage.Watch(func(r usage.Report) { s.broadcastUI(Args{"ev": "usage.changed", "report": r}) })
	ch, cancel := s.cfg.Term.SubscribeAll() // nunca descarta data/exit
	s.cancel = cancel
	go s.pump(ch)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return nil
}

func (s *Server) Close() {
	if s.ln != nil {
		s.ln.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.stopUs != nil {
		s.stopUs()
	}
	s.watch.Close()
	_ = os.Remove(s.cfg.SockPath)
}

// ---- conexões ----

func (c *conn) close() { c.once.Do(func() { close(c.done); c.c.Close() }) }

// send bloqueia (respostas nunca são descartadas); push descarta a conexão lenta.
func (c *conn) send(b []byte) {
	select {
	case c.out <- b:
	case <-c.done:
	}
}

func (c *conn) push(b []byte) {
	select {
	case c.out <- b:
	default:
		c.close() // UI lenta: derruba (reanexa e recebe replay)
	}
}

func line(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(Args{"ok": false, "error": err.Error()})
	}
	return append(b, '\n')
}

func (s *Server) serve(nc net.Conn) {
	c := &conn{c: nc, out: make(chan []byte, 16384), done: make(chan struct{})}
	defer func() { c.close(); s.dropConn(c) }()
	uid, pid, err := auth.Peer(nc)
	if err != nil || !auth.SameUser(uid) {
		return
	}
	c.origin, c.cardID = auth.Classify(pid, s.cfg.DaemonPid, s.cfg.Term.ShellPids())
	go func() {
		for {
			select {
			case b := <-c.out:
				if _, err := nc.Write(b); err != nil {
					c.close()
					return
				}
			case <-c.done:
				return
			}
		}
	}()
	r := bufio.NewReaderSize(nc, 64*1024)
	for {
		l, err := r.ReadBytes('\n')
		if len(l) > 0 {
			s.dispatch(c, l)
		}
		if err != nil {
			return
		}
	}
}

// Comandos que podem bloquear muito: rodam fora da fila da conexão (H1).
// Mantém ordem estrita só em pty.write/resize/send, ws.*, links.set, presets.*.
func isSlow(cmd string) bool {
	switch cmd {
	case "wait", "screenshot", "pty.kill", "pty.killAll", "pty.detachAll", "app.flush", "usage.get", "usage.setSettings":
		return true
	}
	return false // notes.* fica em ordem (N4)
}

func (s *Server) dispatch(c *conn, l []byte) {
	var req struct {
		ID   any    `json:"id"`
		Cmd  string `json:"cmd"`
		Args Args   `json:"args"`
	}
	if err := json.Unmarshal(l, &req); err != nil {
		if len(trimSpace(l)) > 0 {
			c.send(line(Args{"ok": false, "error": "JSON inválido"}))
		}
		return
	}
	if req.Args == nil {
		req.Args = Args{}
	}
	if req.Cmd == "pty.killAll" { // N1: só cards do ws ativo, snapshot antes de ir async
		ids := []string{}
		for _, i := range s.cfg.Term.List() {
			if s.visible(i.ID) {
				ids = append(ids, i.ID)
			}
		}
		req.Args["\x00ids"] = ids
	}
	run := func() {
		defer func() {
			if r := recover(); r != nil { // M9
				log.Printf("panic em %q: %v\n%s", req.Cmd, r, debug.Stack())
				c.send(line(Args{"id": req.ID, "ok": false, "error": fmt.Sprintf("erro interno: %v", r)}))
			}
		}()
		res, err := s.handle(c, req.Cmd, req.Args)
		if err != nil {
			c.send(line(Args{"id": req.ID, "ok": false, "error": err.Error()}))
			return
		}
		c.send(line(Args{"id": req.ID, "ok": true, "result": res}))
	}
	// requisições de uma conexão executam em ordem (teclas, ws.save/ws.load)
	if isSlow(req.Cmd) {
		go run()
	} else {
		run()
	}
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t') {
		b = b[1:]
	}
	return b
}

func (s *Server) dropConn(c *conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.uis {
		if u == c {
			s.uis = append(s.uis[:i:i], s.uis[i+1:]...)
			break
		}
	}
	for rid, p := range s.pendingUI {
		if p.c == c {
			p.ch <- reply{err: "UI não está aberta"}
			delete(s.pendingUI, rid)
		}
	}
}

// caller: card (peer creds ou token) ou usuário (""). Peer creds têm prioridade.
func (s *Server) caller(c *conn, a Args) (string, error) {
	switch c.origin {
	case auth.Card:
		return c.cardID, nil
	case auth.Orphan:
		return "", errors.New("processo órfão do daemon: não autorizado")
	}
	from, _ := a["from"].(string)
	token, _ := a["token"].(string)
	if from == "" && token == "" {
		return "", nil
	}
	if from == "" || !auth.VerifyToken(from, token) {
		return "", errors.New("token inválido: o CLI precisa rodar dentro de um card do Colmeia (COLMEIA_TOKEN)")
	}
	return from, nil
}

// ---- eventos ----

func (s *Server) uiConns() []*conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*conn{}, s.uis...)
}

func (s *Server) broadcastUI(v any) {
	b := line(v)
	for _, u := range s.uiConns() {
		u.push(b)
	}
}

// emit: CanvasEvent para todas as UIs (sem UI: no-op, como sem janela no TS).
func (s *Server) emit(ev Args) { s.broadcastUI(Args{"ev": "canvas", "event": ev}) }

// splitIncomplete separa um sufixo UTF-8 incompleto (fica para o próximo flush).
func splitIncomplete(b []byte) (ok, rest []byte) {
	for i := 1; i <= 3 && i <= len(b); i++ {
		c := b[len(b)-i]
		if c&0xC0 == 0x80 {
			continue
		}
		if c >= 0xC0 && !utf8.FullRune(b[len(b)-i:]) {
			return b[:len(b)-i], b[len(b)-i:]
		}
		break
	}
	return b, nil
}

// chunk: bytes pendentes de um card + offset do primeiro byte (H4).
type chunk struct {
	off  int64
	data []byte
}

func (s *Server) pump(ch <-chan term.Event) {
	for ev := range ch {
		s.pumpOne(ev)
	}
}

func (s *Server) pumpOne(ev term.Event) {
	defer func() {
		if r := recover(); r != nil { // M9
			log.Printf("panic no pump: %v\n%s", r, debug.Stack())
		}
	}()
	switch ev.Kind {
	case "data":
		s.piper.OnData(ev.ID)
		if len(s.uiConns()) == 0 {
			return
		}
		s.sendMu.Lock()
		defer s.sendMu.Unlock()
		s.pmu.Lock()
		c := s.pending[ev.ID]
		if c != nil && c.off+int64(len(c.data)) != ev.Offset {
			// lacuna/sobreposição: emite o pendente cru (inclui cauda UTF-8) e recomeça (L5)
			old := c
			delete(s.pending, ev.ID)
			s.pmu.Unlock()
			if len(old.data) > 0 {
				s.sendData(ev.ID, old.off, old.data)
			}
			s.pmu.Lock()
			c = nil
		}
		if c == nil {
			c = &chunk{off: ev.Offset}
			s.pending[ev.ID] = c
		}
		c.data = append(c.data, ev.Data...)
		if s.flushT == nil {
			s.flushT = time.AfterFunc(8*time.Millisecond, s.flushPending)
		}
		s.pmu.Unlock()
	case "exit":
		s.sendMu.Lock()
		defer s.sendMu.Unlock()
		s.flushLocked()
		s.pmu.Lock()
		rest := s.pending[ev.ID]
		delete(s.pending, ev.ID)
		s.pmu.Unlock()
		if rest != nil && len(rest.data) > 0 {
			s.sendData(ev.ID, rest.off, rest.data)
		}
		s.broadcastUI(Args{"ev": "pty.exit", "id": ev.ID, "code": ev.Code})
	case "resync":
		// assinante atrasado: descarta o pendente e manda a UI reidratar (ensure → replay)
		s.dropPending(ev.ID)
		s.broadcastUI(Args{"ev": "pty.reset", "id": ev.ID})
	}
}

func (s *Server) flushPending() {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	s.flushLocked()
}

// flushLocked: exige sendMu (extrai e envia sem intercalar com outro flush).
func (s *Server) flushLocked() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic no flush: %v", r)
		}
	}()
	type out struct {
		id   string
		off  int64
		data []byte
	}
	var outs []out
	s.pmu.Lock()
	s.flushT = nil
	for id, c := range s.pending {
		ok, rest := splitIncomplete(c.data)
		if len(ok) > 0 {
			outs = append(outs, out{id, c.off, append([]byte{}, ok...)})
		}
		if len(rest) > 0 {
			s.pending[id] = &chunk{off: c.off + int64(len(ok)), data: append([]byte{}, rest...)}
		} else {
			delete(s.pending, id)
		}
	}
	s.pmu.Unlock()
	for _, o := range outs {
		s.sendData(o.id, o.off, o.data)
	}
}

// sendData: len = contagem de bytes crus (a string JSON pode inflar bytes inválidos).
func (s *Server) sendData(id string, off int64, b []byte) {
	s.broadcastUI(Args{"ev": "pty.data", "id": id, "data": string(b), "offset": off, "len": len(b)})
}

// dropPending: descarta pendente de um card (resync).
func (s *Server) dropPending(id string) {
	s.pmu.Lock()
	delete(s.pending, id)
	s.pmu.Unlock()
}

// askUI: pede algo à UI mais recente e espera ui.reply.
func (s *Server) askUI(op string, extra Args) (any, error) {
	s.mu.Lock()
	if len(s.uis) == 0 {
		s.mu.Unlock()
		return nil, errors.New("UI não está aberta")
	}
	u := s.uis[len(s.uis)-1]
	rid := strconv.FormatUint(s.ridSeq.Add(1), 10)
	p := &pendingReq{c: u, ch: make(chan reply, 1)}
	s.pendingUI[rid] = p
	s.mu.Unlock()
	ev := Args{"ev": "ui.req", "rid": rid, "op": op}
	for k, v := range extra {
		ev[k] = v
	}
	u.push(line(ev))
	select {
	case r := <-p.ch:
		if !r.ok {
			return nil, errors.New(r.err)
		}
		return r.result, nil
	case <-time.After(25 * time.Second):
		s.mu.Lock()
		delete(s.pendingUI, rid)
		s.mu.Unlock()
		return nil, errors.New("UI não respondeu")
	}
}

func jsonIndent(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }
