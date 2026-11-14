//go:build windows

package term

import (
	"sync"
	"time"
	"unicode/utf8"
)

// O conhost do Windows 10 descarta os marcadores de bracketed paste (ESC[200~ / ESC[201~) na
// entrada do ConPTY e entrega o texto aos poucos (~1 KB a cada ~75 ms). O programa (Claude Code,
// opencode…) recebe rajadas soltas, trata cada uma como uma colagem separada e pode jogar fora as
// primeiras: um `colmeia send` de 1,1 KB chegava só com os últimos 100 bytes. Em pedaços pequenos
// e espaçados o texto entra inteiro, como digitação; o \n vira quebra de linha no campo de texto.
const (
	pacedChunk = 256                    // bytes por pedaço (cortado em fronteira de runa)
	pacedGap   = 150 * time.Millisecond // intervalo entre pedaços
	pacedEnter = 300 * time.Millisecond // folga antes do Enter final
)

// pacedTail: fim do envio em andamento por card; mantém a ordem entre envios seguidos.
var (
	pacedMu   sync.Mutex
	pacedTail = map[string]chan struct{}{}
)

// sendPaced assume o envio quando o programa pediu bracketed paste e o texto é multilinha ou
// maior que um pedaço. Roda em goroutine: quem chama (CLI, barra de broadcast) não fica preso.
func (m *mgr) sendPaced(id, text string, bracketed, enter bool) bool {
	if !legacyConhost || modernConpty() || !bracketed || (len(text) <= pacedChunk && !containsNL(text)) {
		return false
	}
	done := make(chan struct{})
	pacedMu.Lock()
	prev := pacedTail[id]
	pacedTail[id] = done
	pacedMu.Unlock()
	go func() {
		defer func() {
			pacedMu.Lock()
			if pacedTail[id] == done {
				delete(pacedTail, id)
			}
			pacedMu.Unlock()
			close(done)
		}()
		if prev != nil {
			<-prev
		}
		for i, part := range splitPaced(text, pacedChunk) {
			if i > 0 {
				time.Sleep(pacedGap)
			}
			if m.Write(id, []byte(part)) != nil {
				return // card encerrado no meio do envio
			}
		}
		if enter {
			time.Sleep(pacedEnter)
			_ = m.Write(id, []byte("\r"))
		}
	}()
	return true
}

// splitPaced corta s em pedaços de até max bytes sem partir uma runa UTF-8.
func splitPaced(s string, max int) []string {
	var out []string
	for len(s) > max {
		cut := max
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		if cut == 0 { // sequência inválida mais longa que max: corta cru
			cut = max
		}
		out = append(out, s[:cut])
		s = s[cut:]
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}
