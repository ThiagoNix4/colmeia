//go:build !windows

package term

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// DefaultShell: shell de login do usuário ($SHELL ou /bin/bash).
func DefaultShell() (string, []string) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/bash"
	}
	return sh, []string{"-l"}
}

type unixPTY struct{ f *os.File }

func (p unixPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p unixPTY) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p unixPTY) Close() error                { return p.f.Close() }
func (p unixPTY) Resize(cols, rows int) error {
	return pty.Setsize(p.f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

type unixProc struct{ cmd *exec.Cmd }

func (p *unixProc) Pid() int { return p.cmd.Process.Pid }

// Wait colhe o filho; sinalizado → 128+sinal.
func (p *unixProc) Wait() (int, error) {
	err := p.cmd.Wait()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				code = 128 + int(ws.Signal())
			}
		} else {
			code = -1
		}
	}
	return code, err
}

// Terminate: SIGHUP (ou SIGKILL se hard) no grupo e no pid.
func (p *unixProc) Terminate(hard bool) {
	sig := syscall.SIGHUP
	if hard {
		sig = syscall.SIGKILL
	}
	pid := p.cmd.Process.Pid
	_ = syscall.Kill(-pid, sig)
	_ = syscall.Kill(pid, sig)
}

func startProc(name string, args []string, dir string, env []string, cols, rows int) (ptyIO, procHandle, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, nil, err
	}
	return unixPTY{f}, &unixProc{cmd}, nil
}
