package ui

import (
	"path/filepath"

	"github.com/eugenioenko/ttt/internal/core/buffer"
	"github.com/eugenioenko/ttt/internal/core/cursor"
	"github.com/eugenioenko/ttt/internal/core/selection"
	"github.com/eugenioenko/ttt/internal/view"
)

// LineWindow places a pane's Buf.Lines inside a larger read-only document.
// The pane only uses it for line numbers and the scrollbar; whoever owns the
// document slides the window and handles moves outside it.
type LineWindow struct {
	// Start is the document line of Buf.Lines[0].
	Start int
	// Total is the number of lines in the whole document.
	Total int
	// GoTo moves the cursor to a document line.
	GoTo func(line int)
	// ScrollTo scrolls so a document line is at the top.
	ScrollTo func(top int)
}

func (e *EditorPaneWidget) lineNumberBase() int {
	if e.Window != nil {
		return e.Window.Start
	}
	return 0
}

func (e *EditorPaneWidget) documentLines() int {
	if e.Window != nil {
		return max(e.Window.Total, e.Window.Start+len(e.Buf.Lines))
	}
	return len(e.Buf.Lines)
}

// OpenWindowTab opens a read-only tab showing lines of a larger document. It
// replaces any other tab for the same path.
func (g *EditorGroupWidget) OpenWindowTab(path, title string, lines []string, window *LineWindow) {
	if len(lines) == 0 {
		lines = []string{""}
	}
	if title == "" {
		title = filepath.Base(path)
	}
	tab := editorTab{
		FilePath: path,
		IconPath: path,
		Title:    title,
		Buf:      &buffer.Buffer{Lines: lines},
		Cur:      &cursor.Cursor{},
		Vp:       &view.Viewport{},
		Undo:     g.newUndoStack(),
		Sel:      &selection.Selection{},
		TabSize:  g.TabSize,
		ReadOnly: true,
		Virtual:  true,
		Window:   window,
	}
	for i := range g.tabs {
		t := &g.tabs[i]
		if t.FilePath != path || t.Content != nil || t.Diff != nil {
			continue
		}
		if g.OnFileClose != nil && t.Highlighter != nil && !t.Virtual {
			g.OnFileClose(t.FilePath, t.Highlighter.Language())
		}
		tab.ID = t.ID
		*t = tab
		g.SwitchTab(i)
		g.syncTabs()
		return
	}
	g.tabs = append(g.tabs, tab)
	g.SwitchTab(len(g.tabs) - 1)
}

// SetWindowLines replaces the lines of path's window tab with the document
// lines from start, keeping the cursor, selection and scroll position on the
// same document lines.
func (g *EditorGroupWidget) SetWindowLines(path string, start int, lines []string) {
	t := g.windowTab(path)
	if t == nil {
		return
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	shift := t.Window.Start - start
	t.Window.Start = start
	t.Window.Total = max(t.Window.Total, start+len(lines))
	t.Buf.SetLines(lines)
	last := len(lines) - 1
	clampLine := func(l int) int { return min(max(l+shift, 0), last) }
	t.Cur.Line = clampLine(t.Cur.Line)
	t.Cur.Col = min(t.Cur.Col, len([]rune(lines[t.Cur.Line])))
	t.Vp.TopLine = min(max(t.Vp.TopLine+shift, 0), last)
	if t.Sel.Active {
		t.Sel.Anchor.Line = clampLine(t.Sel.Anchor.Line)
		t.Sel.Anchor.Col = min(t.Sel.Anchor.Col, len([]rune(lines[t.Sel.Anchor.Line])))
	}
	if g.activeTab() == t {
		g.clearSearchState()
	}
}

// SetWindowTotal updates the document length of path's window tab.
func (g *EditorGroupWidget) SetWindowTotal(path string, total int) {
	if t := g.windowTab(path); t != nil {
		t.Window.Total = total
	}
}

// WindowView reports the active window tab's path, the window-relative cursor
// line, top line and viewport height, for deciding when to slide the window.
func (g *EditorGroupWidget) WindowView() (path string, cursorLine, top, height, lines int, ok bool) {
	t := g.activeTab()
	if t == nil || t.Window == nil || t.Content != nil {
		return "", 0, 0, 0, 0, false
	}
	return t.FilePath, t.Cur.Line, t.Vp.TopLine, g.Editor.Viewport.Height, len(t.Buf.Lines), true
}

// ScrollWindowTop scrolls the active window tab so a window line is at the top.
func (g *EditorGroupWidget) ScrollWindowTop(top int) {
	if t := g.activeTab(); t != nil && t.Window != nil {
		t.Vp.TopLine = min(max(top, 0), len(t.Buf.Lines)-1)
	}
}

func (g *EditorGroupWidget) HasWindowTab(path string) bool {
	return g.windowTab(path) != nil
}

// WindowLines returns how many lines path's window tab holds.
func (g *EditorGroupWidget) WindowLines(path string) int {
	if t := g.windowTab(path); t != nil {
		return len(t.Buf.Lines)
	}
	return 0
}

func (g *EditorGroupWidget) windowTab(path string) *editorTab {
	for i := range g.tabs {
		if g.tabs[i].FilePath == path && g.tabs[i].Window != nil {
			return &g.tabs[i]
		}
	}
	return nil
}
