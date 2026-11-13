//go:build !windows

package api

import (
	"net"
	"path/filepath"
	"testing"
)

func testSockPath(t *testing.T) string { return filepath.Join(t.TempDir(), "s.sock") }

func testDial(path string) (net.Conn, error) { return net.Dial("unix", path) }
