package term

// Replay: anel de bytes crus (≤ Ring), cortado após quebra de linha. Escolhido em vez do
// screen serializado do emulador: reproduz idêntico no xterm.js (cores, alt-screen, scrollback)
// e é o que o ptyManager.ts já fazia. O emulador vt só serve ao Read/WaitIdle.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/charmbracelet/x/vt"
)

const historySeparator = "\r\n\x1b[2m[sessão anterior encerrada]\x1b[0m\r\n"

type session struct {
	mu         sync.Mutex
	req        SpawnReq
	meta       Info
	ptmx       ptyIO
	proc       procHandle
	emu        *vt.Emulator
	cols, rows int
	bracketed  bool
	ring       []byte
	alive      bool
	exitCode   *int
	lastOut    time.Time
	lastIn     time.Time
	histTimer  *time.Timer
	removed    bool
	done       chan struct{}
	total      int64 // bytes emitidos no fluxo (inclui a semente do replay)
	created    time.Time
	histMu     sync.Mutex // serializa gravações/remoção do histórico
	waited     bool       // cmd.Wait concluído (pid já colhido)
}

type mgr struct {
	opts Options
	mu   sync.Mutex
	ss   map[string]*session
	subs map[int]*sub
	next int
	idMu map[string]*sync.Mutex // serializa Ensure/Restart/Kill por id
}

type sub struct {
	ch     chan Event
	lagged map[string]bool // ids que perderam eventos; um resync por id
	// all: fila ilimitada (assinante interno)
	all   bool
	qmu   sync.Mutex
	qcond *sync.Cond
	q     []Event
	done  bool
}

func (m *mgr) idLock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.idMu[id]
	if l == nil {
		l = &sync.Mutex{}
		m.idMu[id] = l
	}
	return l
}

func New(opts Options) Manager {
	if opts.HistoryDir == "" {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			h, _ := os.UserHomeDir()
			base = filepath.Join(h, ".config")
		}
		opts.HistoryDir = filepath.Join(base, "colmeia", "history")
	}
	if opts.HistoryCap <= 0 {
		opts.HistoryCap = 512 * 1024
	}
	if opts.Ring <= 0 {
		opts.Ring = opts.HistoryCap
	}
	startReaper()
	return &mgr{opts: opts, ss: map[string]*session{}, subs: map[int]*sub{}, idMu: map[string]*sync.Mutex{}}
}

func (m *mgr) get(id string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ss[id]
}

func (s *session) info() Info {
	i := s.meta
	i.Alive = s.alive
	i.ExitCode = s.exitCode
	i.Persistent = true // o daemon sobrevive ao app
	if s.alive && s.proc != nil {
		i.Pid = s.proc.Pid()
	} else {
		i.Pid = 0
	}
	return i
}

func (m *mgr) Ensure(r SpawnReq) (string, Info, error) {
	l := m.idLock(r.ID)
	l.Lock()
	defer l.Unlock()
	if s := m.get(r.ID); s != nil {
		if r.Cols > 0 && r.Rows > 0 {
			m.Resize(r.ID, r.Cols, r.Rows)
		}
		return s.snapshot()
	}
	saved := m.readHistory(r.ID)
	if saved != "" {
		saved += historySeparator
	}
	s, err := m.spawn(r, []byte(saved))
	if err != nil {
		return "", Info{}, err
	}
	return s.snapshot() // ring atual (semente + saída já produzida) e offset, atômicos
}

// snapshot devolve ring e info (com Offset) sob o mesmo lock.
func (s *session) snapshot() (string, Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.info()
	i.Offset = s.total
	return string(s.ring), i, nil
}

func (m *mgr) spawn(r SpawnReq, seed []byte) (*session, error) {
	cols, rows := r.Cols, r.Rows
	if cols <= 0 {
		cols = 100
	}
	if rows <= 0 {
		rows = 30
	}
	cols, rows = max(cols, 20), max(rows, 5)
	home, _ := os.UserHomeDir()
	cwd := r.Cwd
	if cwd == "" {
		cwd = home
	} else if err := os.MkdirAll(cwd, 0o755); err != nil {
		cwd = home
	}
	name, args := r.Command, r.Args
	if name == "" {
		name, args = DefaultShell()
	}
	var env []string
	if r.Env != nil {
		// ambiente explícito (já filtrado pelo chamador): não herda o do daemon
		keys := make([]string, 0, len(r.Env))
		for k := range r.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			env = append(env, k+"="+r.Env[k])
		}
	} else {
		env = os.Environ()
	}
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor", "COLMEIA_CARD_ID="+r.ID, "COLMEIA_CARD_TITLE="+r.Title)
	spawnMu.RLock() // o reaper não colhe filhos ainda não registrados
	ptmx, proc, err := startProc(name, args, cwd, env, cols, rows)
	if err != nil {
		spawnMu.RUnlock()
		return nil, fmt.Errorf("spawn %q: %w", name, err)
	}
	track(proc.Pid())
	spawnMu.RUnlock()
	emu := vt.NewEmulator(cols, rows)
	emu.SetScrollbackSize(5000)
	s := &session{
		req: r, meta: Info{ID: r.ID, Preset: r.Preset, Title: r.Title, Cwd: cwd},
		ptmx: ptmx, proc: proc, emu: emu, cols: cols, rows: rows,
		ring: append([]byte(nil), seed...), alive: true, lastOut: time.Now(), done: make(chan struct{}),
		total: int64(len(seed)), created: time.Now(),
	}
	m.mu.Lock()
	m.ss[r.ID] = s
	m.mu.Unlock()
	// o emulador responde a queries (DSR etc.) pelo pipe de leitura: drena e descarta
	// (o xterm.js do renderer já responde; duplicar confundiria o app)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := emu.Read(buf); err != nil {
				return
			}
		}
	}()
	emu.SetCallbacks(vt.Callbacks{Title: func(t string) { m.emit(Event{Kind: "title", ID: r.ID, Data: []byte(t)}) }})
	go m.readLoop(s)
	return s, nil
}

func (m *mgr) readLoop(s *session) {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.lastOut = time.Now()
			_, _ = s.emu.Write(chunk)
			on, off := lastIndex(chunk, "\x1b[?2004h"), lastIndex(chunk, "\x1b[?2004l")
			if on != off {
				s.bracketed = on > off
			}
			s.ring = append(s.ring, chunk...)
			if len(s.ring) > m.opts.Ring+m.opts.Ring/4 {
				s.ring = append([]byte(nil), trimToLimit(s.ring, m.opts.Ring)...)
			}
			start := s.total
			s.total += int64(n)
			m.scheduleFlush(s)
			s.mu.Unlock()
			m.emit(Event{Kind: "data", ID: s.meta.ID, Data: chunk, Offset: start})
		}
		if err != nil {
			break
		}
	}
	code, _ := s.proc.Wait()
	untrack(s.proc.Pid())
	_ = s.ptmx.Close()
	s.mu.Lock()
	s.alive = false
	s.exitCode = &code
	removed := s.removed
	s.mu.Unlock()
	m.flush(s)
	close(s.done)
	if !removed {
		m.emit(Event{Kind: "exit", ID: s.meta.ID, Code: code})
	}
}

func lastIndex(b []byte, sub string) int {
	for i := len(b) - len(sub); i >= 0; i-- {
		if string(b[i:i+len(sub)]) == sub {
			return i
		}
	}
	return -1
}

// scheduleFlush: no máx. 1 gravação a cada 10s por card. Chamar com s.mu preso.
func (m *mgr) scheduleFlush(s *session) {
	if s.histTimer != nil || s.removed {
		return
	}
	s.histTimer = time.AfterFunc(10*time.Second, func() {
		s.mu.Lock()
		s.histTimer = nil
		s.mu.Unlock()
		m.flush(s)
	})
}

// flush grava o histórico agora, cancelando o timer. Chamar SEM s.mu; a E/S de disco
// roda fora do lock da sessão (só histMu), então não trava leitura/escrita do PTY.
func (m *mgr) flush(s *session) {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	s.mu.Lock()
	if s.histTimer != nil {
		s.histTimer.Stop()
		s.histTimer = nil
	}
	if s.removed {
		s.mu.Unlock()
		return
	}
	snap := append([]byte(nil), s.ring...)
	s.mu.Unlock()
	m.writeHistory(s.meta.ID, snap)
}

// discard marca a sessão como removida e apaga o histórico (depois de qualquer flush em voo).
func (m *mgr) discard(s *session) {
	s.mu.Lock()
	s.removed = true
	if s.histTimer != nil {
		s.histTimer.Stop()
		s.histTimer = nil
	}
	s.mu.Unlock()
	s.terminate()
	s.closeEmu()
	s.histMu.Lock()
	m.removeHistory(s.meta.ID)
	s.histMu.Unlock()
}

func (m *mgr) emit(e Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sb := range m.subs {
		if sb.all {
			sb.qmu.Lock()
			sb.q = append(sb.q, e)
			sb.qmu.Unlock()
			sb.qcond.Signal()
			continue
		}
		// avisa cada id com perda (resync com ID) antes de voltar ao fluxo normal
		for id := range sb.lagged {
			select {
			case sb.ch <- Event{Kind: "resync", ID: id}:
				delete(sb.lagged, id)
			default:
			}
		}
		if len(sb.lagged) > 0 {
			if e.ID != "" {
				sb.lagged[e.ID] = true
			}
			continue
		}
		select {
		case sb.ch <- e:
		default: // assinante lento: descarta, nunca bloqueia o PTY; resync depois
			if e.ID != "" {
				sb.lagged[e.ID] = true
			}
		}
	}
}

func (m *mgr) Subscribe() (<-chan Event, func()) {
	sb := &sub{ch: make(chan Event, 1024), lagged: map[string]bool{}}
	return sb.ch, m.addSub(sb)
}

// SubscribeAll: fila ilimitada com goroutine de entrega; nunca descarta.
func (m *mgr) SubscribeAll() (<-chan Event, func()) {
	sb := &sub{ch: make(chan Event), all: true}
	sb.qcond = sync.NewCond(&sb.qmu)
	go func() {
		defer close(sb.ch)
		for {
			sb.qmu.Lock()
			for len(sb.q) == 0 && !sb.done {
				sb.qcond.Wait()
			}
			if sb.done {
				sb.qmu.Unlock()
				return
			}
			e := sb.q[0]
			sb.q = sb.q[1:]
			sb.qmu.Unlock()
			// entrega bloqueante, mas abandona se cancelado
			for delivered := false; !delivered; {
				sb.qmu.Lock()
				d := sb.done
				sb.qmu.Unlock()
				if d {
					return
				}
				select {
				case sb.ch <- e:
					delivered = true
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	}()
	return sb.ch, m.addSub(sb)
}

func (m *mgr) addSub(sb *sub) func() {
	m.mu.Lock()
	id := m.next
	m.next++
	m.subs[id] = sb
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		_, ok := m.subs[id]
		delete(m.subs, id)
		m.mu.Unlock()
		if !ok {
			return
		}
		if sb.all {
			sb.qmu.Lock()
			sb.done = true
			sb.qmu.Unlock()
			sb.qcond.Broadcast()
		} else {
			close(sb.ch) // emit roda sob m.mu, então já não envia aqui
		}
	}
}

// terminate mata o grupo de processos e espera o fim da leitura.
func (s *session) terminate() {
	s.mu.Lock()
	alive := s.alive
	pid := 0
	if s.proc != nil {
		pid = s.proc.Pid()
	}
	s.mu.Unlock()
	if !alive || pid == 0 {
		return
	}
	s.proc.Terminate(false)
	select {
	case <-s.done:
	case <-time.After(1500 * time.Millisecond):
		s.proc.Terminate(true)
		select {
		case <-s.done:
		case <-time.After(time.Second):
			// neto com setsid segurando o slave: fecha o ptmx para liberar readLoop/fd
			_ = s.ptmx.Close()
		}
	}
}

func (m *mgr) Restart(id string) error {
	l := m.idLock(id)
	l.Lock()
	defer l.Unlock()
	s := m.get(id)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	req, cols, rows, title := s.req, s.cols, s.rows, s.meta.Title
	s.mu.Unlock()
	m.mu.Lock()
	delete(m.ss, id)
	m.mu.Unlock()
	m.discard(s) // não grava histórico nem anuncia exit da sessão antiga
	req.Cols, req.Rows, req.Title = cols, rows, title
	_, err := m.spawn(req, nil)
	return err
}

func (m *mgr) Write(id string, data []byte) error {
	s := m.get(id)
	if s == nil {
		return fmt.Errorf("card não encontrado: %s", id)
	}
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return fmt.Errorf("card encerrado: %s", id)
	}
	s.lastIn = time.Now()
	f := s.ptmx
	s.mu.Unlock()
	_, err := f.Write(data)
	return err
}

func (m *mgr) Send(id, text string, enter bool) bool {
	s := m.get(id)
	if s == nil {
		return false
	}
	s.mu.Lock()
	alive, bracketed := s.alive, s.bracketed
	s.mu.Unlock()
	if !alive {
		return false
	}
	payload := text
	if bracketed && containsNL(text) {
		payload = "\x1b[200~" + text + "\x1b[201~"
	}
	if m.Write(id, []byte(payload)) != nil {
		return false
	}
	if enter {
		time.AfterFunc(250*time.Millisecond, func() { _ = m.Write(id, []byte("\r")) })
	}
	return true
}

func containsNL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return true
		}
	}
	return false
}

var keys = map[string]string{
	"enter": "\r", "return": "\r", "esc": "\x1b", "escape": "\x1b", "tab": "\t", "shift-tab": "\x1b[Z",
	"ctrl-c": "\x03", "ctrl-d": "\x04", "ctrl-z": "\x1a", "ctrl-l": "\x0c", "ctrl-u": "\x15", "ctrl-a": "\x01", "ctrl-e": "\x05", "ctrl-r": "\x12",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
	"home": "\x1b[H", "end": "\x1b[F", "pgup": "\x1b[5~", "pgdn": "\x1b[6~", "pageup": "\x1b[5~", "pagedown": "\x1b[6~",
	"backspace": "\x7f", "delete": "\x1b[3~", "space": " ",
}

func (m *mgr) Key(id, key string) error {
	seq, ok := keys[key]
	if !ok {
		return fmt.Errorf("tecla desconhecida: %s", key)
	}
	return m.Write(id, []byte(seq))
}

func (m *mgr) Resize(id string, cols, rows int) {
	s := m.get(id)
	if s == nil || cols < 2 || rows < 2 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cols == cols && s.rows == rows {
		return
	}
	s.cols, s.rows = cols, rows
	s.emu.Resize(cols, rows)
	if s.alive {
		_ = s.ptmx.Resize(cols, rows)
	}
}

func (m *mgr) SetMeta(id, title string) {
	if s := m.get(id); s != nil && title != "" {
		s.mu.Lock()
		s.meta.Title = title
		s.mu.Unlock()
	}
}

func (m *mgr) Kill(id string) error {
	l := m.idLock(id)
	l.Lock()
	defer l.Unlock()
	m.mu.Lock()
	s := m.ss[id]
	delete(m.ss, id)
	m.mu.Unlock()
	if s == nil {
		m.removeHistory(id)
		return nil
	}
	m.discard(s)
	return nil
}

func (m *mgr) Detach(id string) {
	if s := m.get(id); s != nil {
		m.flush(s)
	}
}

func (m *mgr) FlushAll() {
	m.mu.Lock()
	all := make([]*session, 0, len(m.ss))
	for _, s := range m.ss {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		m.flush(s)
	}
}

// Prune apaga históricos e mata sessões vivas fora de keep (recém-criadas ficam, evita corrida com o UI).
func (m *mgr) Prune(keep []string) {
	set := map[string]bool{}
	for _, id := range keep {
		set[id] = true
	}
	m.mu.Lock()
	var kill []string
	for id, s := range m.ss {
		if !set[id] && time.Since(s.created) > 30*time.Second {
			kill = append(kill, id)
		}
	}
	m.mu.Unlock()
	for _, id := range kill {
		_ = m.Kill(id)
	}
	m.pruneHistory(keep)
}

func (m *mgr) List() []Info {
	m.mu.Lock()
	all := make([]*session, 0, len(m.ss))
	for _, s := range m.ss {
		all = append(all, s)
	}
	m.mu.Unlock()
	out := make([]Info, 0, len(all))
	for _, s := range all {
		s.mu.Lock()
		out = append(out, s.info())
		s.mu.Unlock()
	}
	return out
}

func (m *mgr) Get(id string) (Info, bool) {
	s := m.get(id)
	if s == nil {
		return Info{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info(), true
}

func (m *mgr) ShellPids() map[int]string {
	out := map[int]string{}
	for _, i := range m.List() {
		if i.Alive && i.Pid > 0 {
			out[i.Pid] = i.ID
		}
	}
	return out
}

// Read: scrollback + tela renderizados como texto puro, sem linhas vazias no fim.
func (m *mgr) Read(id string, lines int) (string, error) {
	s := m.get(id)
	if s == nil {
		return "", fmt.Errorf("card não encontrado: %s", id)
	}
	if lines <= 0 {
		lines = 60
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	w := s.emu.Width()
	for y := 0; y < s.emu.ScrollbackLen(); y++ {
		out = append(out, rowText(w, func(x int) string { return cellStr(s.emu.ScrollbackCellAt(x, y)) }))
	}
	for y := 0; y < s.emu.Height(); y++ {
		out = append(out, rowText(w, func(x int) string { return cellStr(s.emu.CellAt(x, y)) }))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) > lines {
		out = out[len(out)-lines:]
	}
	res := ""
	for i, l := range out {
		if i > 0 {
			res += "\n"
		}
		res += l
	}
	return res, nil
}

func (m *mgr) WaitIdle(id string, idle, timeout time.Duration) (bool, time.Duration) {
	start := time.Now()
	for {
		s := m.get(id)
		if s == nil {
			return false, time.Since(start)
		}
		s.mu.Lock()
		now := time.Now()
		responded := !s.lastOut.Before(s.lastIn) || now.Sub(s.lastIn) > max(idle*3, 15*time.Second)
		ok := !s.alive || (responded && now.Sub(s.lastOut) >= idle)
		s.mu.Unlock()
		if ok {
			return true, time.Since(start)
		}
		if time.Since(start) >= timeout {
			return false, time.Since(start)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// closeEmu encerra o dreno via pipe (emu.Close() corre com emu.Read no flag interno).
func (s *session) closeEmu() {
	if pw, ok := s.emu.InputPipe().(*io.PipeWriter); ok {
		_ = pw.CloseWithError(io.EOF)
	}
}
