package api

import (
	"testing"
	"time"
)

func TestPresetNewRmNoDeadlock(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	srv := New(Config{Term: fakeTerm{}, SockPath: testSockPath(t), DaemonPid: 999999})
	done := make(chan error, 1)
	go func() {
		r, err := srv.handleCLI("", "preset.new", Args{"title": "Apis-A", "tool": "claude", "model": "haiku", "worker": true})
		if err != nil {
			done <- err
			return
		}
		id, _ := r.(Args)["id"].(string)
		_, err = srv.handleCLI("", "preset.rm", Args{"id": id})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock em preset.new/preset.rm")
	}
}

func TestScanTailBashPromptWithCommand(t *testing.T) {
	text := "u@h:~/x$ make old\nerror: velho\nu@h:~/x$ make\nok\nu@h:~/x$"
	if _, e := scanTail(text); e {
		t.Fatal("erro de comando anterior vazou para a execução limpa")
	}
	if _, e := scanTail("u@h:~/x$ make\nerror: x\nu@h:~/x$"); !e {
		t.Fatal("erro do comando atual deveria contar")
	}
}
