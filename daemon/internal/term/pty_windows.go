//go:build windows

package term

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

// DefaultShell: $COLMEIA_SHELL, senão pwsh.exe, senão powershell.exe.
func DefaultShell() (string, []string) {
	if sh := os.Getenv("COLMEIA_SHELL"); sh != "" {
		return sh, []string{"-NoLogo"}
	}
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		return p, []string{"-NoLogo"}
	}
	return "powershell.exe", []string{"-NoLogo"}
}

type winPTY struct{ c *conpty.ConPty }

// legacyConhost: conhost do Windows 10 / Server até 2022 (o do Windows 11 trata SI/SO como controle).
var legacyConhost = func() bool {
	_, _, build := windows.RtlGetNtVersionNumbers()
	return build < 22000
}()

func (p winPTY) Read(b []byte) (int, error) {
	n, err := p.c.Read(b)
	if legacyConhost {
		blankControlCells(b[:n])
	}
	return n, err
}

// blankControlCells troca por espaço os bytes de controle que o conhost antigo devolve como célula.
// Com a página de código UTF-8 (Claude Code, opencode…), um SI (0x0F) escrito pelo programa vira uma
// célula no buffer do conhost: o cursor dele anda uma coluna e o byte volta cru na saída do ConPTY.
// O xterm.js (e o emulador do daemon) ignoram o byte, então tudo que vem depois na linha sai uma
// coluna à esquerda e o que é apagado fica na tela até a linha ser repintada. Espaço ocupa a mesma
// célula que o conhost contou; os controles que ele emite de verdade (BEL, BS, TAB, LF, CR, ESC) ficam.
// Troca de 1 byte por 1 byte, todos < 0x80: não quebra UTF-8 nem muda o tamanho do chunk.
func blankControlCells(b []byte) {
	for i, c := range b {
		switch {
		case c == 0x07, c == 0x08, c == 0x09, c == 0x0a, c == 0x0d, c == 0x1b:
		case c < 0x20, c == 0x7f:
			b[i] = ' '
		}
	}
}

func (p winPTY) Write(b []byte) (int, error) { return p.c.Write(b) }
func (p winPTY) Close() error                { return p.c.Close() }
func (p winPTY) Resize(cols, rows int) error { return p.c.Resize(cols, rows) }

type winProc struct {
	pid    int
	h      windows.Handle // processo
	job    windows.Handle // job com KILL_ON_JOB_CLOSE: mata a árvore
	closed bool
	soft   func() // fase suave (Ctrl-C via ConPTY)
	mu     sync.Mutex
}

func (p *winProc) Pid() int { return p.pid }

func (p *winProc) Wait() (int, error) {
	if _, err := windows.WaitForSingleObject(p.h, windows.INFINITE); err != nil {
		return -1, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.h, &code); err != nil {
		return -1, err
	}
	// o processo acabou: sobras da árvore caem junto com o job
	p.Terminate(true)
	p.mu.Lock()
	_ = windows.CloseHandle(p.h) // L2: sem vazar o handle do processo
	p.h = 0
	p.mu.Unlock()
	if code != 0 {
		return int(code), fmt.Errorf("exit status %d", code)
	}
	return 0, nil
}

// Terminate: mata o job inteiro (não há SIGHUP no Windows; hard só difere no fallback).
func (p *winProc) Terminate(hard bool) {
	if !hard {
		// M5: fase suave = Ctrl-C no console; o manager chama hard após 1,5s
		if p.soft != nil {
			p.soft()
		}
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job != 0 && !p.closed {
		_ = windows.TerminateJobObject(p.job, 1)
		if hard {
			_ = windows.CloseHandle(p.job)
			p.closed = true
		}
		return
	}
	if p.h != 0 {
		_ = windows.TerminateProcess(p.h, 1)
	}
}

func newJob() windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0
	}
	return job
}

func startProc(name string, args []string, dir string, env []string, cols, rows int) (ptyIO, procHandle, error) {
	cp, err := conpty.New(cols, rows, 0)
	if err != nil {
		return nil, nil, err
	}
	// conpty.Spawn junta nome relativo com attr.Dir (semântica de syscall.StartProcess)
	// e nunca consulta o PATH: "powershell.exe" virava <cwd>\powershell.exe.
	if filepath.Base(name) == name {
		if lp, err := exec.LookPath(name); err == nil {
			name = lp
		}
	}
	argv := append([]string{name}, args...)
	// ComposeCommandLine (padrão do conpty) escapa cada arg no estilo CommandLineToArgvW
	pid, handle, err := cp.Spawn(name, argv, &syscall.ProcAttr{Dir: dir, Env: env})
	if err != nil {
		_ = cp.Close()
		return nil, nil, err
	}
	p := &winProc{pid: pid, h: windows.Handle(handle)}
	p.soft = func() { _, _ = cp.Write([]byte{0x03}) }
	// H2: o ConPTY só dá EOF ao leitor quando fechado; fecha após o filho sair
	// (pequena folga para o leitor drenar o que restou no pipe)
	go func(h windows.Handle) {
		_, _ = windows.WaitForSingleObject(h, windows.INFINITE)
		time.Sleep(300 * time.Millisecond)
		_ = cp.Close()
	}(p.h)
	if job := newJob(); job != 0 {
		if err := windows.AssignProcessToJobObject(job, p.h); err != nil {
			_ = windows.CloseHandle(job)
		} else {
			p.job = job
		}
	}
	return winPTY{cp}, p, nil
}
