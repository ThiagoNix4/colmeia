// Package term: sessões PTY, emulador vt, replay e histórico em disco (Contract A).
package term

import "time"

type SpawnReq struct {
	ID, Preset, Title, Cwd, Command string
	Args                            []string
	Env                             map[string]string
	Cols, Rows                      int
}

type Info struct {
	ID, Preset, Title, Cwd string
	Alive                  bool
	ExitCode               *int
	Pid                    int
	Persistent             bool
	// Offset: bytes totais emitidos até o fim do replay devolvido por Ensure (0 nos demais usos).
	Offset int64
}

// Event.Kind: "data" | "exit" | "title" | "resync".
// Offset (data): posição do início do chunk no fluxo da sessão (mesma base de Info.Offset do Ensure).
// "resync": o assinante ficou para trás e perdeu eventos; refaça Ensure e descarte data com fim ≤ Offset.
type Event struct {
	Kind   string
	ID     string
	Data   []byte
	Code   int
	Offset int64
}

type Manager interface {
	Ensure(r SpawnReq) (replay string, info Info, err error)
	Restart(id string) error
	Write(id string, data []byte) error
	Send(id, text string, enter bool) bool
	Key(id, key string) error
	Resize(id string, cols, rows int)
	SetMeta(id, title string)
	Kill(id string) error
	Detach(id string)
	FlushAll()
	Prune(keep []string) // apaga históricos e mata sessões vivas fora de keep (exceto criadas há <30s)
	List() []Info
	Get(id string) (Info, bool)
	Read(id string, lines int) (string, error)
	WaitIdle(id string, idle, timeout time.Duration) (idleOK bool, waited time.Duration)
	ShellPids() map[int]string
	// Subscribe: assinante externo (UI). Canal limitado; se encher, descarta e depois emite "resync".
	Subscribe() (<-chan Event, func())
	// SubscribeAll: assinante interno; fila ilimitada, nunca perde data/exit.
	SubscribeAll() (<-chan Event, func())
}

type Options struct {
	HistoryDir string // default ~/.config/colmeia/history
	HistoryCap int    // default 512KB
	Ring       int    // bytes do ring de replay; default = HistoryCap
}
