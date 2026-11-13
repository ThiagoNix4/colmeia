//go:build windows

package notes

import (
	"strings"
	"testing"
)

func TestObsidianFileAppData(t *testing.T) {
	t.Setenv("APPDATA", `C:\x\Roaming`)
	if f := obsidianFile(); !strings.EqualFold(f, `C:\x\Roaming\obsidian\obsidian.json`) {
		t.Fatal(f)
	}
}
