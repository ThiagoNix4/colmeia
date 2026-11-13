package term

import (
	"os"
	"strings"
	"testing"
	"time"
)

func spawnSh(t *testing.T) (Manager, string) {
	dir := t.TempDir()
	m := New(Options{HistoryDir: dir})
	t.Cleanup(func() { _ = m.Kill("c1") })
	if _, _, err := m.Ensure(SpawnReq{ID: "c1", Command: "sh", Cwd: dir, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	return m, dir
}

func TestSendRead(t *testing.T) {
	m, _ := spawnSh(t)
	if !m.Send("c1", "echo hi-$((20+22))", true) {
		t.Fatal("send")
	}
	for i := 0; i < 60; i++ {
		if out, _ := m.Read("c1", 20); strings.Contains(out, "hi-42") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, _ := m.Read("c1", 20)
	t.Fatalf("read sem hi-42: %q", out)
}

func TestWaitIdle(t *testing.T) {
	m, _ := spawnSh(t)
	ok, _ := m.WaitIdle("c1", 300*time.Millisecond, 5*time.Second)
	if !ok {
		t.Fatal("não ficou ocioso")
	}
	m.Send("c1", "sleep 3", true)
	if ok, _ := m.WaitIdle("c1", 5*time.Second, 400*time.Millisecond); ok {
		t.Fatal("deveria estourar timeout")
	}
}

func TestKillRemovesHistory(t *testing.T) {
	m, dir := spawnSh(t)
	m.Send("c1", "echo x", true)
	time.Sleep(400 * time.Millisecond)
	m.Detach("c1")
	if _, err := os.Stat(dir + "/c1.log"); err != nil {
		t.Fatal("histórico não gravado:", err)
	}
	_ = m.Kill("c1")
	if _, err := os.Stat(dir + "/c1.log"); err == nil {
		t.Fatal("histórico não removido")
	}
	if _, ok := m.Get("c1"); ok {
		t.Fatal("sessão ainda existe")
	}
}
