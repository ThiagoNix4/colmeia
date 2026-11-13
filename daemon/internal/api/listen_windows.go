//go:build windows

package api

import (
	"fmt"
	"net"
	"os/user"
	"time"

	"github.com/Microsoft/go-winio"
)

// listenSock: named pipe com ACL só do usuário atual; pipe respondendo = outro backend.
func listenSock(path string) (net.Listener, error) {
	if c, err := winio.DialPipe(path, ptrDur(500*time.Millisecond)); err == nil {
		c.Close()
		return nil, fmt.Errorf("pipe %s em uso por outro backend do Colmeia", path)
	}
	cfg := &winio.PipeConfig{MessageMode: false, InputBufferSize: 64 * 1024, OutputBufferSize: 64 * 1024}
	// L3: sem SID do usuário não há ACL restrita → falha fechada (nunca a DACL padrão)
	u, err := user.Current()
	if err != nil || u.Uid == "" {
		return nil, fmt.Errorf("não foi possível obter o SID do usuário para a ACL do pipe: %v", err)
	}
	cfg.SecurityDescriptor = "D:P(A;;GA;;;" + u.Uid + ")"
	return winio.ListenPipe(path, cfg)
}

func ptrDur(d time.Duration) *time.Duration { return &d }
