//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"colmeia/daemon/internal/term"

	"golang.org/x/sys/unix"
)

// acquireLock: flock exclusivo no pidfile; o arquivo trancado precisa ser o do disco (N6).
func acquireLock(pidf string) (*os.File, error) {
	f, err := os.OpenFile(pidf, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, errBusy
	}
	var a, b unix.Stat_t
	if unix.Fstat(int(f.Fd()), &a) == nil && unix.Stat(pidf, &b) == nil && a.Ino == b.Ino && a.Dev == b.Dev {
		return f, nil
	}
	f.Close()
	return nil, errBusy // pidfile trocado
}

func writePid(lock *os.File, _ string, pid int) {
	_ = lock.Truncate(0)
	_, _ = lock.WriteAt([]byte(strconv.Itoa(pid)), 0)
}

// clearPid: não remove (outra instância pode já ter o inode).
func clearPid(lock *os.File, _ string) { _ = lock.Truncate(0) }

// setupProcess: órfãos dos cards são reparentados ao daemon (peer creds os rejeita).
func setupProcess() { _ = unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) }

// defaultSock: $XDG_RUNTIME_DIR/colmeia.sock (ou tmp).
func defaultSock() string {
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		rt = os.TempDir()
	}
	return filepath.Join(rt, "colmeia.sock")
}

// notifyStop: no unix a parada vem por SIGTERM.
func notifyStop(chan<- os.Signal) {}

func stopPid(pid int, _ string) error { return syscall.Kill(pid, syscall.SIGTERM) }

// killChildren: SIGHUP (suave) ou SIGKILL no grupo e no pid de cada card vivo.
func killChildren(infos []term.Info, hard bool) {
	sig := syscall.SIGHUP
	if hard {
		sig = syscall.SIGKILL
	}
	for _, i := range infos {
		if i.Alive && i.Pid > 0 {
			_ = syscall.Kill(-i.Pid, sig)
			_ = syscall.Kill(i.Pid, sig)
		}
	}
}
