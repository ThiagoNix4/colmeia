package term

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
)

var unsafeName = regexp.MustCompile(`[^\w.-]+`)

func safeName(n string) string {
	n = unsafeName.ReplaceAllString(n, "_")
	if len(n) > 64 {
		n = n[:64]
	}
	if n == "" {
		n = "default"
	}
	return n
}

func (m *mgr) histPath(id string) string {
	return filepath.Join(m.opts.HistoryDir, safeName(id)+".log")
}

// trimToLimit mantém só o fim; começa após uma quebra de linha (evita meio de escape/UTF-8).
func trimToLimit(b []byte, limit int) []byte {
	if len(b) <= limit {
		return b
	}
	t := b[len(b)-limit:]
	if i := indexByte(t, '\n'); i >= 0 && i < 8192 {
		return t[i+1:]
	}
	// sem quebra próxima: pula continuações UTF-8 para não iniciar no meio de um rune
	for len(t) > 0 && t[0]&0xC0 == 0x80 {
		t = t[1:]
	}
	return t
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

var tmpSeq atomic.Uint64

// writeHistory grava atômico (.tmp + rename); falha de disco é ignorada.
func (m *mgr) writeHistory(id string, data []byte) {
	_ = os.MkdirAll(m.opts.HistoryDir, 0o755)
	file := m.histPath(id)
	tmp := file + "." + itoa(os.Getpid()) + "." + itoa(int(tmpSeq.Add(1))) + ".tmp"
	if err := os.WriteFile(tmp, trimToLimit(data, m.opts.HistoryCap), 0o644); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)
	}
}

func (m *mgr) readHistory(id string) string {
	b, err := os.ReadFile(m.histPath(id))
	if err != nil {
		return ""
	}
	return string(b)
}

func (m *mgr) removeHistory(id string) { _ = os.Remove(m.histPath(id)) }

func (m *mgr) pruneHistory(keep []string) {
	set := map[string]bool{}
	for _, id := range keep {
		set[safeName(id)+".log"] = true
	}
	ents, err := os.ReadDir(m.opts.HistoryDir)
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if strings.HasSuffix(n, ".log") && !set[n] {
			_ = os.Remove(filepath.Join(m.opts.HistoryDir, n))
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
