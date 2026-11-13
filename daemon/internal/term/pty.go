package term

import "io"

// ptyIO: lado mestre do pseudo-terminal (pty no unix, ConPTY no Windows).
type ptyIO interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// procHandle: processo filho do card.
type procHandle interface {
	Pid() int
	Wait() (code int, err error)
	Terminate(hard bool) // derruba a árvore (grupo/job); hard = kill forçado
}
