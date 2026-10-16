package app

import (
	"github.com/eugenioenko/ttt/internal/config"
	"github.com/eugenioenko/ttt/internal/fileicons"
	"github.com/eugenioenko/ttt/internal/term"
	"github.com/eugenioenko/ttt/internal/ui"
	"github.com/eugenioenko/ttt/internal/widgets"
)

func setFileIcon(node *widgets.TreeNode) {
	glyph, style := ui.FileIconForPath(config.IconsNerdFont, node.Label)
	node.Icon = glyph
	node.IconStyle = mutedIconStyle(node, style)
}

// setLabelIcon leaves Icon free for rows that already use it, such as the Git
// status letter.
func setLabelIcon(node *widgets.TreeNode, name string) {
	glyph, style := ui.FileIconForPath(config.IconsNerdFont, name)
	node.LabelIcon = glyph
	node.LabelIconStyle = mutedIconStyle(node, style)
}

func fileIconStyle(color fileicons.Color) term.Style {
	return ui.FileIconStyle(color)
}

func mutedIconStyle(node *widgets.TreeNode, style term.Style) term.Style {
	if node.Muted {
		return term.StyleMuted
	}
	return style
}
