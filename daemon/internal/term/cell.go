package term

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

func cellStr(c *uv.Cell) string {
	if c == nil {
		return " "
	}
	if c.Width == 0 && c.Content == "" {
		return ""
	}
	if c.Content == "" {
		return " "
	}
	return c.Content
}

func rowText(w int, at func(x int) string) string {
	var b strings.Builder
	for x := 0; x < w; x++ {
		b.WriteString(at(x))
	}
	return strings.TrimRight(b.String(), " ")
}
