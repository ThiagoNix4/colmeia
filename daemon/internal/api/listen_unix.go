//go:build !windows

package api

import (
	"fmt"
	"net"
	"os"
	"time"
)

// listenSock: socket unix 0600; socket vivo = outro backend (não rouba).
func listenSock(path string) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
		c.Close()
		return nil, fmt.Errorf("socket %s em uso por outro backend do Colmeia", path)
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return ln, nil
}
