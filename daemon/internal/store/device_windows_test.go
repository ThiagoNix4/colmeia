//go:build windows

package store

import "testing"

func TestSafeNameDevice(t *testing.T) {
	if SafeName("con") != "_con" || SafeName("NUL.x") != "_NUL.x" || SafeName("ok") != "ok" {
		t.Fatal(SafeName("con"), SafeName("NUL.x"))
	}
}
