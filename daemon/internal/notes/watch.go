package notes

import (
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher observa a pasta de notas do workspace ativo e emite note-changed (debounce 150ms).
type Watcher struct {
	mu     sync.Mutex
	w      *fsnotify.Watcher
	ws     string
	seen   map[string]float64
	timers map[string]*time.Timer
	emit   func(id string, mtime float64)
}

func NewWatcher(emit func(id string, mtime float64)) *Watcher {
	return &Watcher{seen: map[string]float64{}, timers: map[string]*time.Timer{}, emit: emit}
}

// MarkSeen: mtime já conhecido (escrita do próprio renderer não gera evento).
func (x *Watcher) MarkSeen(id string, mtime float64) {
	x.mu.Lock()
	x.seen[id] = mtime
	x.mu.Unlock()
}

// Announce: escrita via CLI anuncia na hora (o watch depois dedupe pelo mtime).
func (x *Watcher) Announce(id string, mtime float64) {
	x.MarkSeen(id, mtime)
	x.emit(id, mtime)
}

func (x *Watcher) Watch(ws string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if ws == x.ws && x.w != nil {
		return
	}
	if x.w != nil {
		x.w.Close()
		x.w = nil
	}
	x.ws = ws
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	if err := w.Add(NoteDir(ws)); err != nil {
		w.Close()
		return
	}
	x.w = w
	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				x.schedule(ws, filepath.Base(ev.Name))
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

func (x *Watcher) schedule(ws, name string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if t := x.timers[name]; t != nil {
		t.Stop()
	}
	x.timers[name] = time.AfterFunc(150*time.Millisecond, func() {
		defer func() { // M9
			if r := recover(); r != nil {
				log.Printf("panic no watcher: %v", r)
			}
		}()
		x.mu.Lock()
		delete(x.timers, name)
		x.mu.Unlock()
		for _, n := range List(ws) {
			if filepath.Base(n.File) != name {
				continue
			}
			m := Mtime(n.File)
			x.mu.Lock()
			same := x.seen[n.ID] == m
			if !same {
				x.seen[n.ID] = m
			}
			x.mu.Unlock()
			if !same {
				x.emit(n.ID, m)
			}
			return
		}
	})
}

func (x *Watcher) Close() {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.w != nil {
		x.w.Close()
		x.w = nil
	}
}
