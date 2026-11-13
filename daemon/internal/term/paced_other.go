//go:build !windows

package term

// sendPaced: só existe no Windows (conhost antigo); aqui o Send segue o caminho normal.
func (m *mgr) sendPaced(id, text string, bracketed, enter bool) bool { return false }
