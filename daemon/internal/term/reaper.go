//go:build !windows

package term

// Reaper: o daemon é subreaper; netos órfãos viram filhos dele e ficam zumbis.
// Colhe todo zumbi filho que não seja um shell rastreado (esses têm cmd.Wait).
// Só colhe zumbis vistos em duas varreduras seguidas, para não roubar o Wait de
// outros exec.Cmd do daemon (que colhem em milissegundos).

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	spawnMu    sync.RWMutex // spawn (leitura) × varredura do reaper (escrita)
	trackMu    sync.Mutex
	tracked    = map[int]struct{}{}
	reaperOnce sync.Once
)

func track(pid int) {
	trackMu.Lock()
	tracked[pid] = struct{}{}
	trackMu.Unlock()
}

func untrack(pid int) {
	trackMu.Lock()
	delete(tracked, pid)
	trackMu.Unlock()
}

func startReaper() {
	reaperOnce.Do(func() {
		go func() {
			prev := map[int]bool{}
			for range time.Tick(5 * time.Second) {
				prev = reapOnce(prev)
			}
		}()
	})
}

// reapOnce colhe os zumbis órfãos vistos também na rodada anterior e devolve os candidatos atuais.
func reapOnce(prev map[int]bool) map[int]bool {
	spawnMu.Lock()
	defer spawnMu.Unlock()
	cur := map[int]bool{}
	me := os.Getpid()
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return cur
	}
	trackMu.Lock()
	defer trackMu.Unlock()
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if _, ok := tracked[pid]; ok {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// formato: pid (comm) S ppid ...; comm pode ter espaços/parênteses
		str := string(b)
		i := strings.LastIndexByte(str, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(str[i+1:])
		if len(f) < 2 || f[0] != "Z" {
			continue
		}
		if ppid, _ := strconv.Atoi(f[1]); ppid != me {
			continue
		}
		cur[pid] = true
		if prev[pid] {
			var ws syscall.WaitStatus
			_, _ = syscall.Wait4(pid, &ws, syscall.WNOHANG, nil)
			delete(cur, pid)
		}
	}
	return cur
}
