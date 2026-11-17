//go:build windows

package api

import (
	"os/exec"
	"syscall"
)

// hideWindow: o daemon roda sem console; um processo de console filho (ex.: opencode.cmd do npm)
// abriria uma janela de terminal que pisca na tela. CREATE_NO_WINDOW evita isso.
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
