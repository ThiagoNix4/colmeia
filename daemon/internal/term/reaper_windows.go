package term

import "sync"

// Windows: sem zumbis nem subreaper; mantém só a API usada pelo manager.
var spawnMu sync.RWMutex

func track(pid int)   {}
func untrack(pid int) {}
func startReaper()    {}
