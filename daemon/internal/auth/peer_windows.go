//go:build windows

package auth

import "net"

// Windows: sem SO_PEERCRED; a barreira é a ACL do named pipe (só o usuário atual) + token.
// pid 0 → Classify devolve External (exige from+token).
func Peer(c net.Conn) (uid uint32, pid int, err error) { return 0, 0, nil }

// SameUser: o pipe já é restrito ao usuário pela ACL.
func SameUser(uid uint32) bool { return true }

// Ppid: sem /proc no Windows.
func Ppid(pid int) int { return 0 }
