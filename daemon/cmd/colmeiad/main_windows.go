//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"colmeia/daemon/internal/term"

	"golang.org/x/sys/windows"
)

// acquireLock: abre o arquivo de lock sem compartilhamento; falha = já em uso.
// O pidfile é separado (leitura por --stop não pode colidir com o lock).
func acquireLock(pidf string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(pidf + ".lock")
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errBusy
		}
		return nil, err
	}
	return os.NewFile(uintptr(h), pidf+".lock"), nil
}

func writePid(_ *os.File, pidf string, pid int) {
	tmp := pidf + ".tmp"
	if os.WriteFile(tmp, []byte(strconv.Itoa(pid)), 0o644) == nil {
		_ = os.Rename(tmp, pidf)
	}
}

func clearPid(_ *os.File, pidf string) { _ = os.Remove(pidf) }

func setupProcess() {}

const stopEvent = `Local\colmeiad-stop`

// defaultSock: named pipe `\\.\pipe\colmeia-<usuário>` (contrato).
func defaultSock() string {
	name := os.Getenv("USERNAME")
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
		if i := strings.LastIndex(name, `\`); i >= 0 {
			name = name[i+1:]
		}
	}
	return `\\.\pipe\colmeia-` + name
}

// notifyStop: evento nomeado → SIGTERM no canal (shutdown graceful: flush, pid limpo).
func notifyStop(ch chan<- os.Signal) {
	p, _ := windows.UTF16PtrFromString(stopEvent)
	ev, err := windows.CreateEvent(nil, 0, 0, p)
	if err != nil {
		return
	}
	go func() {
		if r, _ := windows.WaitForSingleObject(ev, windows.INFINITE); r == windows.WAIT_OBJECT_0 {
			ch <- syscall.SIGTERM
		}
	}()
}

// stopPid: pede parada graceful pelo evento e espera o lock soltar; só então cai para taskkill,
// e apenas se o pid for mesmo um colmeiad (pidfile velho pode apontar para pid reaproveitado).
func stopPid(pid int, pidf string) error {
	if f, err := acquireLock(pidf); err == nil { // lock livre = daemon não está rodando
		f.Close()
		return errors.New("não está rodando")
	}
	if p, err := windows.UTF16PtrFromString(stopEvent); err == nil {
		if ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, p); err == nil {
			_ = windows.SetEvent(ev)
			_ = windows.CloseHandle(ev)
			for i := 0; i < 50; i++ {
				if f, err := acquireLock(pidf); err == nil {
					f.Close()
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	if !isColmeiad(pid) {
		return errors.New("pid não é colmeiad")
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

func isColmeiad(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH*2)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return false
	}
	return strings.EqualFold(filepath.Base(windows.UTF16ToString(buf[:n])), "colmeiad.exe")
}

// killChildren: os cards já morrem com o job (KILL_ON_JOB_CLOSE); taskkill garante a árvore.
func killChildren(infos []term.Info, hard bool) {
	if !hard {
		return
	}
	for _, i := range infos {
		if i.Alive && i.Pid > 0 {
			_ = exec.Command("taskkill", "/PID", strconv.Itoa(i.Pid), "/T", "/F").Run()
		}
	}
}
