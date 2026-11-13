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
)

// errBusy: outra instância segura o lock.
var errBusy = errors.New("lock em uso")

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
		if err != nil || pid <= 0 || stopPid(pid, pidf) != nil {
			fmt.Fprintln(os.Stderr, "colmeiad não está rodando")
			os.Exit(1)
		}
		return
	}

	// instância única: lock do pidfile (flock no unix, abertura exclusiva no Windows)
	_ = os.MkdirAll(cfgD, 0o755)
	var lock *os.File
	for i := 0; i < 50; i++ { // ~5s: o daemon antigo pode estar encerrando
		f, err := acquireLock(pidf)
		if err == nil {
			lock = f
			break
		}
		if !errors.Is(err, errBusy) { // L9: erro real (permissão etc.), não "já rodando"
			fmt.Fprintln(os.Stderr, "pidfile:", err)
			os.Exit(1)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lock == nil {
		fmt.Fprintln(os.Stderr, "colmeiad já está rodando")
		os.Exit(0)
	}

	setupProcess()

	if *sock == "" {
		*sock = os.Getenv("COLMEIA_SOCK_PATH")
	}
	if *sock == "" {
		*sock = defaultSock()
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
	writePid(lock, pidf, os.Getpid())
	fmt.Println("colmeiad pronto:", *sock, "pid", os.Getpid())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	notifyStop(ch) // Windows: evento de parada graceful (sem SIGTERM)
	<-ch

	// shutdown: histórico em disco, depois derruba os filhos (sem apagar histórico)
	srv.Close()
	tm.FlushAll()
	infos := tm.List()
	killChildren(infos, false)
	time.Sleep(500 * time.Millisecond)
	killChildren(infos, true)
	tm.FlushAll()
	clearPid(lock, pidf)
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
