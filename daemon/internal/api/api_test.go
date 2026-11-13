package api

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"

	"colmeia/daemon/internal/auth"
	"colmeia/daemon/internal/term"
)

type fakeTerm struct{ term.Manager }

func (fakeTerm) Prune([]string)                         {}
func (fakeTerm) List() []term.Info                      { return nil }
func (fakeTerm) ShellPids() map[int]string              { return nil }
func (fakeTerm) Subscribe() (<-chan term.Event, func()) { return make(chan term.Event), func() {} }
func (fakeTerm) SubscribeAll() (<-chan term.Event, func()) {
	return make(chan term.Event), func() {}
}
func (fakeTerm) Get(string) (term.Info, bool) { return term.Info{}, false }

func call(t *testing.T, c net.Conn, r *bufio.Reader, cmd string, args Args) map[string]any {
	t.Helper()
	b, _ := json.Marshal(Args{"id": 1, "cmd": cmd, "args": args})
	_, _ = c.Write(append(b, '\n'))
	l, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(l, &out)
	return out
}

func TestServerPingAndAttach(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	sock := testSockPath(t)
	srv := New(Config{Term: fakeTerm{}, SockPath: sock, DaemonPid: 999999})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	c, err := testDial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := bufio.NewReader(c)
	if res := call(t, c, r, "ping", nil); res["result"].(map[string]any)["pong"] != "pong" {
		t.Fatalf("ping: %v", res)
	}
	if res := call(t, c, r, "ui.attach", Args{"secret": "errado"}); res["ok"] != false {
		t.Fatalf("attach errado: %v", res)
	}
	if res := call(t, c, r, "ui.attach", Args{"secret": auth.SecretHex()}); res["ok"] != true {
		t.Fatalf("attach: %v", res)
	}
	if res := call(t, c, r, "ws.list", nil); res["ok"] != true {
		t.Fatalf("ws.list: %v", res)
	}
}
