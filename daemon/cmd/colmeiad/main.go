// colmeiad: daemon do Colmeia (PTYs + estado + socket Contract B).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"colmeia/daemon/internal/api"
	"colmeia/daemon/internal/auth"
	"colmeia/daemon/internal/term"

	"golang.org/x/sys/unix"
)

func firstExisting(cands ...string) string {
	for _, c := range cands {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return cands[0]
}

func main() {
	var (
		sock = flag.String("sock", "", "caminho do socket (padrão: $COLMEIA_SOCK_PATH ou $XDG_RUNTIME_DIR/colmeia.sock)")
		docs = flag.String("docs", os.Getenv("COLMEIA_DOCS_DIR"), "pasta docs/ (ORCHESTRATOR.md)")
		cli  = flag.String("cli", os.Getenv("COLMEIA_CLI_DIR"), "pasta cli/ (colmeia, statusline)")
		stop = flag.Bool("stop", false, "para o daemon em execução (SIGTERM)")
		ver  = flag.Bool("version", false, "imprime a versão (hash do binário) e sai")
		cfgD = auth.ConfigDir()
		pidf = filepath.Join(cfgD, "colmeiad.pid")
	)
	flag.Parse()

	version := binVersion()
	if *ver {
		fmt.Println(version)
		return
	}

	if *stop {
		b, err := os.ReadFile(pidf)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || pid <= 0 || syscall.Kill(pid, syscall.SIGTERM) != nil {
			fmt.Fprintln(os.Stderr, "colmeiad não está rodando")
			os.Exit(1)
		}
		return
	}

	// instância única: flock no pidfile
	_ = os.MkdirAll(cfgD, 0o755)
	var lock *os.File
	var lerr error
	for i := 0; i < 50; i++ { // ~5s: o daemon antigo pode estar encerrando
		f, err := os.OpenFile(pidf, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pidfile:", err)
			os.Exit(1)
		}
		if lerr = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); lerr == nil {
			// N6: o arquivo trancado precisa ser o que está no disco
			var a, b unix.Stat_t
			if unix.Fstat(int(f.Fd()), &a) == nil && unix.Stat(pidf, &b) == nil && a.Ino == b.Ino && a.Dev == b.Dev {
				lock = f
				break
			}
			lerr = errors.New("pidfile trocado")
		}
		f.Close()
		time.Sleep(100 * time.Millisecond)
	}
	if lock == nil {
		fmt.Fprintln(os.Stderr, "colmeiad já está rodando")
		os.Exit(0)
	}

	// órfãos dos cards são reparentados ao daemon (peer creds os rejeita)
	_ = unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)

	if *sock == "" {
		*sock = os.Getenv("COLMEIA_SOCK_PATH")
	}
	if *sock == "" {
		rt := os.Getenv("XDG_RUNTIME_DIR")
		if rt == "" {
			rt = os.TempDir()
		}
		*sock = filepath.Join(rt, "colmeia.sock")
	}
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	wd, _ := os.Getwd()
	if *docs == "" {
		*docs = firstExisting(filepath.Join(wd, "docs"), filepath.Join(exeDir, "..", "docs"), filepath.Join(exeDir, "..", "..", "docs"))
	}
	if *cli == "" {
		*cli = firstExisting(filepath.Join(wd, "cli"), filepath.Join(exeDir, "..", "cli"), filepath.Join(exeDir, "..", "..", "cli"))
	}

	tm := term.New(term.Options{})
	if a, err := filepath.Abs(*docs); err == nil && *docs != "" {
		*docs = a
	}
	if a, err := filepath.Abs(*cli); err == nil && *cli != "" {
		*cli = a
	}
	srv := api.New(api.Config{Term: tm, SockPath: *sock, DocsDir: *docs, CliDir: *cli, DaemonPid: os.Getpid(), Version: version})
	if err := srv.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "socket:", err)
		os.Exit(1)
	}
	// pid só depois do bind do socket: quem perde não suja o pidfile
	_ = lock.Truncate(0)
	_, _ = lock.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	fmt.Println("colmeiad pronto:", *sock, "pid", os.Getpid())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	<-ch

	// shutdown: histórico em disco, depois derruba os filhos (sem apagar histórico)
	srv.Close()
	tm.FlushAll()
	infos := tm.List()
	for _, i := range infos {
		if i.Alive && i.Pid > 0 {
			_ = syscall.Kill(-i.Pid, syscall.SIGHUP)
			_ = syscall.Kill(i.Pid, syscall.SIGHUP)
		}
	}
	time.Sleep(500 * time.Millisecond)
	for _, i := range infos {
		if i.Alive && i.Pid > 0 {
			_ = syscall.Kill(-i.Pid, syscall.SIGKILL)
			_ = syscall.Kill(i.Pid, syscall.SIGKILL)
		}
	}
	tm.FlushAll()
	_ = lock.Truncate(0) // N6: não remove (outra instância pode já ter o inode)
}

// binVersion: sha256 (12 hex) do executável — muda a cada build.
func binVersion() string {
	exe, err := os.Executable()
	if err != nil {
		return "dev"
	}
	f, err := os.Open(exe)
	if err != nil {
		return "dev"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "dev"
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
