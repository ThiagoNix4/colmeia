//go:build !windows

package api

import "os/exec"

// hideWindow: só o Windows abre janela de console para processos filhos.
func hideWindow(*exec.Cmd) {}
