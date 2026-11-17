//go:build !windows

package api

// freshPath: no unix o shell de login (-l) dos cards já recarrega o PATH do perfil.
func freshPath(inherited string) string { return inherited }
