//go:build windows

package api

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/Microsoft/go-winio"
)

// named pipe único por teste (o daemon real usa \\.\pipe\colmeia-<usuário>)
func testSockPath(t *testing.T) string {
	return fmt.Sprintf(`\\.\pipe\colmeia-test-%d-%s`, os.Getpid(), t.Name())
}

func testDial(path string) (net.Conn, error) { return winio.DialPipe(path, nil) }
