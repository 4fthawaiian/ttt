package ui

import (
	"path/filepath"

	"github.com/eugenioenko/ttt/internal/fileicons"
	"github.com/eugenioenko/ttt/internal/icons"
	"github.com/eugenioenko/ttt/internal/term"
	"github.com/eugenioenko/ttt/internal/textwidth"
)

func FileIconForPath(mode, path string) (string, term.Style) {
	if path == "" || !icons.IsNerd(mode) {
		return "", term.StyleDefault
	}
	icon := fileicons.ForFile(filepath.Base(path))
	return icon.Glyph, FileIconStyle(icon.Color)
}

func fileIconPrefixWidth(mode, path string) int {
	glyph, _ := FileIconForPath(mode, path)
	if glyph == "" {
		return 0
	}
	return textwidth.String(glyph) + 1
}

type fileIconRender struct {
	mode     string
	path     string
	bgStyle  term.Style
	selected bool
	bold     bool
}

func drawFileIcon(surface Surface, x, y, right int, icon fileIconRender) int {
	glyph, iconStyle := FileIconForPath(icon.mode, icon.path)
	if glyph == "" {
		return x
	}
	if icon.selected {
		iconStyle = icon.bgStyle
	}
	if x+textwidth.String(glyph) > right {
		return x
	}
	for _, ch := range glyph {
		w := textwidth.Rune(ch)
		if x+w > right {
			return x
		}
		surface.SetCell(x, y, term.Cell{Ch: ch, Style: iconStyle, BgStyle: icon.bgStyle, Bold: icon.bold})
		x += w
	}
	if x < right {
		surface.SetCell(x, y, term.Cell{Ch: ' ', Style: icon.bgStyle})
		x++
	}
	return x
}

func FileIconStyle(color fileicons.Color) term.Style {
	switch color {
	case fileicons.ColorRed:
		return term.StyleFileIconRed
	case fileicons.ColorYellow:
		return term.StyleFileIconYellow
	case fileicons.ColorGreen:
		return term.StyleFileIconGreen
	case fileicons.ColorCyan:
		return term.StyleFileIconCyan
	case fileicons.ColorBlue:
		return term.StyleFileIconBlue
	case fileicons.ColorMagenta:
		return term.StyleFileIconMagenta
	default:
		return term.StyleDefault
	}
}
