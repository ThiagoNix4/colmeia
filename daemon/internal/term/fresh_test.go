package term

// FreshBefore só é enviado pelo spawn para cards de TUI que retomam a própria conversa (opencode).

import (
	"testing"
	"time"
)

// App reaberto: sessão viva de antes de FreshBefore é recriada (pid novo); a criada depois é mantida.
func TestEnsureFreshBefore(t *testing.T) {
	m, dir := spawnSh(t)
	req := SpawnReq{ID: "c1", Command: testShell(), Args: testShellArgs(), Cwd: dir, Cols: 80, Rows: 24}
	old, _ := m.Get("c1")

	_, _, _ = m.Ensure(req) // sem FreshBefore: reaproveita
	if same, _ := m.Get("c1"); same.Pid != old.Pid {
		t.Fatalf("sem FreshBefore não deveria recriar: %d → %d", old.Pid, same.Pid)
	}

	time.Sleep(50 * time.Millisecond) // relógio do Windows: resolução de alguns ms
	req.FreshBefore = time.Now()
	if _, _, err := m.Ensure(req); err != nil {
		t.Fatal(err)
	}
	fresh, _ := m.Get("c1")
	if !fresh.Alive || fresh.Pid == old.Pid {
		t.Fatalf("deveria recriar: vivo=%v pid %d → %d", fresh.Alive, old.Pid, fresh.Pid)
	}

	_, _, _ = m.Ensure(req) // a nova é posterior a FreshBefore: mantém
	if again, _ := m.Get("c1"); again.Pid != fresh.Pid {
		t.Fatalf("sessão nova não deveria ser recriada de novo: %d → %d", fresh.Pid, again.Pid)
	}
}
