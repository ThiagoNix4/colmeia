//go:build windows

package term

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ConPTY moderno (Microsoft.Windows.Console.ConPTY, MIT): conpty.dll + OpenConsole.exe ao lado do
// colmeiad.exe (ou em $COLMEIA_CONPTY_DIR). Diferente do conhost embutido no Windows 10, ele repassa
// ao xterm os pedidos do app (mouse tracking, alt-screen). Qualquer falha → cai no kernel32.
// COLMEIA_CONPTY=inbox força o ConPTY do sistema.

var (
	modernOnce                              sync.Once
	modernCreate, modernResize, modernClose *windows.LazyProc
	modernOK                                bool
)

// modernConpty carrega a dll uma vez; false = usar o ConPTY do sistema.
func modernConpty() bool {
	modernOnce.Do(func() {
		if strings.EqualFold(os.Getenv("COLMEIA_CONPTY"), "inbox") || runtime.GOARCH != "amd64" {
			return
		}
		dir := os.Getenv("COLMEIA_CONPTY_DIR")
		if dir == "" {
			exe, err := os.Executable()
			if err != nil {
				return
			}
			dir = filepath.Dir(exe)
		}
		dll, oc := filepath.Join(dir, "conpty.dll"), filepath.Join(dir, "OpenConsole.exe")
		if _, err := os.Stat(dll); err != nil {
			return // não empacotado: comportamento antigo, sem ruído
		}
		if _, err := os.Stat(oc); err != nil {
			log.Printf("conpty moderno: OpenConsole.exe ausente em %s; usando ConPTY do sistema", dir)
			return
		}
		l := windows.NewLazyDLL(dll)
		if err := l.Load(); err != nil {
			log.Printf("conpty moderno: %v; usando ConPTY do sistema", err)
			return
		}
		c, r, cl := l.NewProc("ConptyCreatePseudoConsole"), l.NewProc("ConptyResizePseudoConsole"), l.NewProc("ConptyClosePseudoConsole")
		for _, p := range []*windows.LazyProc{c, r, cl} {
			if err := p.Find(); err != nil {
				log.Printf("conpty moderno: %v; usando ConPTY do sistema", err)
				return
			}
		}
		modernCreate, modernResize, modernClose, modernOK = c, r, cl, true
		log.Printf("conpty moderno: %s", dll)
	})
	return modernOK
}

// coord empacota COORD (2×int16) em um único argumento, como a ABI x64 passa a struct.
func coord(cols, rows int) uintptr { return uintptr(uint16(cols)) | uintptr(uint16(rows))<<16 }

type modernPTY struct {
	hpc               windows.Handle
	inRead, inWrite   windows.Handle
	outRead, outWrite windows.Handle
	attr              *windows.ProcThreadAttributeListContainer
	once              sync.Once
}

func newModernPTY(cols, rows int) (*modernPTY, error) {
	sec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	p := &modernPTY{}
	if err := windows.CreatePipe(&p.inRead, &p.inWrite, sec, 0); err != nil {
		return nil, fmt.Errorf("pipe de entrada: %w", err)
	}
	if err := windows.CreatePipe(&p.outRead, &p.outWrite, sec, 0); err != nil {
		_ = windows.CloseHandle(p.inRead)
		_ = windows.CloseHandle(p.inWrite)
		return nil, fmt.Errorf("pipe de saída: %w", err)
	}
	hr, _, _ := modernCreate.Call(coord(cols, rows), uintptr(p.inRead), uintptr(p.outWrite), 0, uintptr(unsafe.Pointer(&p.hpc)))
	if int32(hr) < 0 {
		p.closePipes()
		return nil, fmt.Errorf("ConptyCreatePseudoConsole: 0x%x", uint32(hr))
	}
	attr, err := windows.NewProcThreadAttributeList(1)
	if err == nil {
		err = attr.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&p.hpc)), unsafe.Sizeof(p.hpc))
	}
	if err != nil {
		if attr != nil {
			attr.Delete()
		}
		_, _, _ = modernClose.Call(uintptr(p.hpc))
		p.closePipes()
		return nil, err
	}
	p.attr = attr
	return p, nil
}

func (p *modernPTY) closePipes() {
	for _, h := range []windows.Handle{p.inRead, p.inWrite, p.outRead, p.outWrite} {
		_ = windows.CloseHandle(h)
	}
}

func (p *modernPTY) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(p.outRead, b, &n, nil)
	return int(n), err
}

func (p *modernPTY) Write(b []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(p.inWrite, b, &n, nil)
	return int(n), err
}

func (p *modernPTY) Resize(cols, rows int) error {
	hr, _, _ := modernResize.Call(uintptr(p.hpc), coord(cols, rows))
	if int32(hr) < 0 {
		return fmt.Errorf("ConptyResizePseudoConsole: 0x%x", uint32(hr))
	}
	return nil
}

func (p *modernPTY) Close() error {
	var err error
	p.once.Do(func() {
		_ = windows.CloseHandle(p.inRead)
		_ = windows.CloseHandle(p.outWrite)
		if p.attr != nil {
			p.attr.Delete()
		}
		_, _, _ = modernClose.Call(uintptr(p.hpc))
		err = errors.Join(windows.CloseHandle(p.inWrite), windows.CloseHandle(p.outRead))
	})
	return err
}

// envBlock monta o bloco UTF-16 do CreateProcess; garante SystemRoot (sem ele muitos programas falham).
func envBlock(env []string) (*uint16, error) {
	var b []uint16
	hasRoot := false
	for _, kv := range env {
		if strings.IndexByte(kv, 0) >= 0 {
			return nil, syscall.EINVAL
		}
		if len(kv) >= 11 && strings.EqualFold(kv[:11], "SYSTEMROOT=") {
			hasRoot = true
		}
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		b = append(b, u...)
	}
	if !hasRoot {
		if v := os.Getenv("SYSTEMROOT"); v != "" {
			u, _ := windows.UTF16FromString("SYSTEMROOT=" + v)
			b = append(b, u...)
		}
	}
	b = append(b, 0)
	return &b[0], nil
}

// spawn cria o processo ligado ao pseudo-console (name já resolvido pelo chamador).
func (p *modernPTY) spawn(name string, args []string, dir string, env []string) (int, uintptr, error) {
	if dir != "" && !filepath.IsAbs(name) {
		abs, err := filepath.Abs(name)
		if err != nil {
			return 0, 0, err
		}
		name = abs
	}
	argv0, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, 0, err
	}
	cmd, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{name}, args...)))
	if err != nil {
		return 0, 0, err
	}
	var dirp *uint16
	if dir != "" {
		if dirp, err = windows.UTF16PtrFromString(dir); err != nil {
			return 0, 0, err
		}
	}
	if env == nil {
		env = os.Environ()
	}
	eb, err := envBlock(env)
	if err != nil {
		return 0, 0, err
	}
	si := new(windows.StartupInfoEx)
	si.Flags = windows.STARTF_USESTDHANDLES
	si.ProcThreadAttributeList = p.attr.List()
	si.Cb = uint32(unsafe.Sizeof(*si))
	pi := new(windows.ProcessInformation)
	sec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	if err := windows.CreateProcess(argv0, cmd, sec, sec, false, flags, eb, dirp, &si.StartupInfo, pi); err != nil {
		return 0, 0, fmt.Errorf("failed to create process: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)
	return int(pi.ProcessId), uintptr(pi.Process), nil
}
