package ui

import (
	"path/filepath"
	"testing"

	"github.com/eugenioenko/ttt/internal/config"
	"github.com/eugenioenko/ttt/internal/fileicons"
	"github.com/eugenioenko/ttt/internal/term"
)

func TestFileIconForPathUsesBasenameAndThemeStyle(t *testing.T) {
	for _, tc := range []struct {
		path  string
		style term.Style
	}{
		{path: "/repo/Dockerfile", style: term.StyleFileIconBlue},
		{path: "/repo/app.test.ts", style: term.StyleFileIconCyan},
		{path: "/repo/file.unknownext", style: term.StyleDefault},
	} {
		t.Run(tc.path, func(t *testing.T) {
			glyph, style := FileIconForPath(config.IconsNerdFont, tc.path)
			if want := fileicons.ForFile(filepath.Base(tc.path)).Glyph; glyph != want {
				t.Fatalf("glyph = %q, want %q", glyph, want)
			}
			if style != tc.style {
				t.Fatalf("style = %v, want %v", style, tc.style)
			}
		})
	}

	if glyph, style := FileIconForPath(config.IconsNone, "/repo/main.go"); glyph != "" || style != term.StyleDefault {
		t.Fatalf("icons disabled = (%q, %v), want empty/default", glyph, style)
	}
	if glyph, style := FileIconForPath(config.IconsNerdFont, ""); glyph != "" || style != term.StyleDefault {
		t.Fatalf("empty path = (%q, %v), want empty/default", glyph, style)
	}
}
