package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func windowLines(start, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("doc line %d", start+i+1)
	}
	return lines
}

func openWindowGroup(t *testing.T, start int) (*EditorGroupWidget, *LineWindow, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := NewEditorGroupWidget(nil, 4, false, "extended")
	w := &LineWindow{Start: start, Total: 100000}
	g.OpenWindowTab(path, "", windowLines(start, 100), w)
	return g, w, path
}

func TestWindowGutterShowsDocumentLineNumbers(t *testing.T) {
	g, _, _ := openWindowGroup(t, 5000)
	e := g.Editor
	e.LineNumbers = true
	e.SetRect(Rect{W: 40, H: 5})
	grid := makeGrid(40, 5)
	e.Render(NewRenderSurface(grid, Rect{W: 40, H: 5}))

	if text := gridText(grid); !strings.Contains(text, "5001") || !strings.Contains(text, "doc line 5001") {
		t.Fatalf("gutter does not show document line numbers:\n%s", text)
	}
}

func TestSetWindowLinesKeepsCursorOnTheSameDocumentLine(t *testing.T) {
	g, w, path := openWindowGroup(t, 1000)
	g.GoToWindowLine(51)

	g.SetWindowLines(path, 1040, windowLines(1040, 100))

	if line, _ := g.ActiveCursor(); line != 1050 {
		t.Errorf("document cursor line = %d, want 1050", line)
	}
	if w.Start != 1040 {
		t.Errorf("window start = %d, want 1040", w.Start)
	}
	if got := g.Editor.Buf.Lines[g.Editor.Cursor.Line]; got != "doc line 1051" {
		t.Errorf("cursor is on %q, want doc line 1051", got)
	}
}

func TestWindowMovesOutsideTheWindowGoToTheOwner(t *testing.T) {
	g, w, _ := openWindowGroup(t, 0)
	var targets []int
	w.GoTo = func(line int) { targets = append(targets, line) }

	g.GoToLine(70000)
	g.Editor.MoveToFileEdge(true, false)
	g.Editor.MoveToFileEdge(false, false)

	if fmt.Sprint(targets) != "[69999 99999 0]" {
		t.Errorf("GoTo targets = %v, want [69999 99999 0]", targets)
	}
}

func TestOpenFileDoesNotReloadAWindowTab(t *testing.T) {
	g, _, path := openWindowGroup(t, 0)

	g.OpenFile(path)

	if got := g.Editor.Buf.Lines[0]; got != "doc line 1" {
		t.Errorf("first line = %q, want the window kept", got)
	}
}

func TestWindowTabNeverWraps(t *testing.T) {
	g, _, _ := openWindowGroup(t, 0)
	g.WordWrap = true
	g.syncTabs()
	if g.Editor.WordWrap {
		t.Error("window tab wraps")
	}
}
