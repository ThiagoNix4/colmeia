// Package auth: token hmac por card + credenciais do peer no socket (SO_PEERCRED).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// ConfigDir: ~/.config/colmeia (ou $XDG_CONFIG_HOME/colmeia).
func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".config")
	}
	return filepath.Join(base, "colmeia")
}

func secretFile() string { return filepath.Join(ConfigDir(), "secret") }

var (
	mu     sync.Mutex
	secret []byte
	hexStr string
)

func readSecret() ([]byte, string, bool) {
	b, err := os.ReadFile(secretFile())
	if err != nil {
		return nil, "", false
	}
	t := strings.TrimSpace(string(b))
	if len(t) < 32 {
		return nil, "", false
	}
	raw, err := hex.DecodeString(t)
	if err != nil {
		raw = []byte(t) // como Buffer.from(t,'hex') tolerante: usa os bytes crus
	}
	return raw, t, true
}

func load() []byte {
	mu.Lock()
	defer mu.Unlock()
	if secret != nil {
		return secret
	}
	if s, h, ok := readSecret(); ok {
		secret, hexStr = s, h
		return secret
	}
	_ = os.MkdirAll(ConfigDir(), 0o755)
	fresh := make([]byte, 32)
	_, _ = rand.Read(fresh)
	h := hex.EncodeToString(fresh)
	f, err := os.OpenFile(secretFile(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_, _ = f.WriteString(h)
		f.Close()
		secret, hexStr = fresh, h
		return secret
	}
	// outro processo criou primeiro: lê o dele
	if s, hh, ok := readSecret(); ok {
		secret, hexStr = s, hh
		return secret
	}
	secret, hexStr = fresh, h
	return secret
}

// SecretHex: conteúdo de ~/.config/colmeia/secret (para ui.attach).
func SecretHex() string {
	load()
	mu.Lock()
	defer mu.Unlock()
	return hexStr
}

// CheckSecret compara em tempo constante o segredo enviado pela UI.
func CheckSecret(s string) bool {
	return hmac.Equal([]byte(strings.TrimSpace(s)), []byte(SecretHex()))
}

func TokenFor(cardID string) string {
	m := hmac.New(sha256.New, load())
	m.Write([]byte(cardID))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

func VerifyToken(cardID string, token any) bool {
	t, ok := token.(string)
	if !ok {
		return false
	}
	return hmac.Equal([]byte(TokenFor(cardID)), []byte(t))
}

// Peer: uid e pid do outro lado de uma conexão unix.
func Peer(c net.Conn) (uid uint32, pid int, err error) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return 0, 0, net.ErrClosed
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, 0, err
	}
	var cred *unix.Ucred
	var cerr error
	if err := raw.Control(func(fd uintptr) {
		cred, cerr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, 0, err
	}
	if cerr != nil {
		return 0, 0, cerr
	}
	return cred.Uid, int(cred.Pid), nil
}

// Ppid lê o pai de um pid em /proc/<pid>/stat (0 se não achar).
func Ppid(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(s[i+1:]) // state ppid ...
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}

// Origin classifica o processo remoto.
//
// Risco residual (M2): a identificação por peer creds só vale para descendentes do daemon.
// Um processo fora da árvore (systemd-run --user, cron) cai em External e, sem
// token, é tratado como usuário; e quem lê ~/.config/colmeia/secret (mesmo uid) forja tokens.
// A fronteira de segurança é o uid, como no modelo anterior.
type Origin int

const (
	External Origin = iota // fora da árvore do daemon: usa from+token
	Card                   // descendente de um shell de card
	Orphan                 // descendente do daemon sem card (reparentado)
)

// Classify sobe a cadeia de ppid de pid; shells = pid do shell → id do card.
func Classify(pid, daemonPid int, shells map[int]string) (Origin, string) {
	for i := 0; pid > 1 && i < 64; i++ {
		if id, ok := shells[pid]; ok {
			return Card, id
		}
		if pid == daemonPid {
			return Orphan, ""
		}
		pid = Ppid(pid)
	}
	return External, ""
}
