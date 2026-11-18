package api

import (
	"testing"

	"colmeia/daemon/internal/store"
)

func TestCheckPresetModel(t *testing.T) {
	s := &Server{presets: []store.Preset{
		{ID: "a", Command: "opencode mini -m opencode-go-gpt-6-luna", Tool: "opencode", Model: "opencode-go-gpt-6-luna"},
		{ID: "b", Command: "opencode -m opencode-go/gpt-6-luna", Tool: "opencode", Model: "opencode-go/gpt-6-luna"},
		{ID: "c", Command: "claude --model haiku"},
	}}
	if s.checkPresetModel("a", "") == nil {
		t.Fatal("modelo sem provider/ deveria ser recusado")
	}
	if err := s.checkPresetModel("b", ""); err != nil {
		t.Fatal(err)
	}
	if s.checkPresetModel("b", "luna") == nil {
		t.Fatal("override inválido deveria falhar")
	}
	if err := s.checkPresetModel("c", ""); err != nil {
		t.Fatal(err)
	}
}
