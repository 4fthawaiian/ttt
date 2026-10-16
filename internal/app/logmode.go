package app

import (
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/gdamore/tcell/v3"

	"github.com/eugenioenko/ttt/internal/core/logfile"
	"github.com/eugenioenko/ttt/internal/ui"
	"github.com/eugenioenko/ttt/internal/view"
)

const (
	logWindowLines = 20000
	// logWindowMargin must stay larger than any single move inside the window
	// (a page, a wheel step), so the window slides before the cursor reaches
	// its edge.
	logWindowMargin = 2000
)

type logView struct {
	file   *logfile.File
	window *ui.LineWindow
	// follow is set while the cursor is on the last line of a fully indexed
	// file, so appended lines keep the view at the end.
	follow  bool
	pending atomic.Bool
}

type LogFileProgress struct {
	Path string
}

func (a *App) OpenActiveInLogMode() {
	a.openLogView(a.EditorGroup.ActiveFilePath())
}

func (a *App) ExplorerOpenInLogMode() {
	a.openLogView(a.explorerNodePath())
}

func (a *App) openLogView(path string) {
	if path == "" {
		a.StatusWarn("No file to open in log mode")
		return
	}
	if buf := a.EditorGroup.BufferForPath(path); buf != nil && buf.Dirty {
		a.StatusWarn(filepath.Base(path) + " has unsaved changes")
		return
	}
	if old := a.logViews[path]; old != nil {
		old.file.Close()
	}
	lv := &logView{}
	file, err := logfile.Open(path, func() {
		if lv.pending.CompareAndSwap(false, true) && a.Screen != nil {
			a.Screen.PostEvent(tcell.NewEventInterrupt(&LogFileProgress{Path: path}))
		}
	})
	if err != nil {
		a.StatusWarn(fmt.Sprintf("Failed to open %s: %v", filepath.Base(path), err))
		return
	}
	lv.file = file
	lv.window = &ui.LineWindow{
		Total:    file.Lines(),
		GoTo:     func(line int) { a.logGoTo(path, line) },
		ScrollTo: func(top int) { a.logScrollTo(path, top) },
	}
	a.logViews[path] = lv
	a.EditorGroup.OpenWindowTab(path, filepath.Base(path)+" (log)", file.ReadLines(0, logWindowLines), lv.window)
	a.SyncWatched()
	a.syncLogStatus()
}

func (a *App) handleLogProgress(ev *LogFileProgress) {
	lv := a.logViews[ev.Path]
	if lv == nil {
		return
	}
	lv.pending.Store(false)
	total := lv.file.Lines()

	if lv.file.TakeReplaced() {
		a.StatusNotify(filepath.Base(ev.Path) + " was truncated or replaced; reading it again")
		lv.window.Total = total
		a.EditorGroup.SetWindowLines(ev.Path, 0, lv.file.ReadLines(0, logWindowLines))
		a.syncLogStatus()
		return
	}

	oldTotal := lv.window.Total
	a.EditorGroup.SetWindowTotal(ev.Path, total)
	_, _, _, _, _, active := a.activeLogWindow(ev.Path)
	switch {
	case total == oldTotal:
	case lv.follow && active:
		start := max(0, total-logWindowLines)
		a.EditorGroup.SetWindowLines(ev.Path, start, lv.file.ReadLines(start, logWindowLines))
		a.EditorGroup.GoToWindowLine(total - lv.window.Start)
	case a.EditorGroup.WindowLines(ev.Path) < logWindowLines:
		// The window has room for lines that were not on disk or not
		// indexed when it was read.
		a.EditorGroup.SetWindowLines(ev.Path, lv.window.Start, lv.file.ReadLines(lv.window.Start, logWindowLines))
	}
	a.syncLogStatus()
}

// syncLogWindow slides the active log tab's window when the cursor or the
// view comes near either of its edges.
func (a *App) syncLogWindow() {
	path, cur, top, h, n, ok := a.EditorGroup.WindowView()
	if !ok {
		return
	}
	lv := a.logViews[path]
	if lv == nil {
		return
	}
	start := lv.window.Start
	lv.follow = start+cur >= lv.window.Total-1 && !lv.file.Indexing()
	switch {
	case start > 0 && (cur < logWindowMargin || top < logWindowMargin):
		a.recenterLog(path, lv, start+min(cur, top))
	case start+n < lv.window.Total && (cur > n-logWindowMargin || top+h > n-logWindowMargin):
		a.recenterLog(path, lv, start+max(cur, top+h))
	}
}

func (a *App) recenterLog(path string, lv *logView, line int) {
	start := min(max(line-logWindowLines/2, 0), max(0, lv.window.Total-logWindowLines))
	if start == lv.window.Start {
		return
	}
	a.EditorGroup.SetWindowLines(path, start, lv.file.ReadLines(start, logWindowLines))
}

func (a *App) logGoTo(path string, line int) {
	lv := a.logViews[path]
	if lv == nil {
		return
	}
	line = min(max(line, 0), lv.window.Total-1)
	if !a.logWindowHolds(lv, line) {
		a.recenterLog(path, lv, line)
	}
	a.EditorGroup.GoToWindowLine(line - lv.window.Start + 1)
}

func (a *App) logScrollTo(path string, top int) {
	lv := a.logViews[path]
	if lv == nil {
		return
	}
	top = min(max(top, 0), lv.window.Total-1)
	if !a.logWindowHolds(lv, top) {
		a.recenterLog(path, lv, top)
	}
	a.EditorGroup.ScrollWindowTop(top - lv.window.Start)
}

func (a *App) logWindowHolds(lv *logView, line int) bool {
	n := a.EditorGroup.WindowLines(lv.file.Path())
	lo, hi := lv.window.Start+logWindowMargin, lv.window.Start+n-logWindowMargin
	if lv.window.Start == 0 {
		lo = 0
	}
	if lv.window.Start+n >= lv.window.Total {
		hi = lv.window.Total
	}
	return line >= lo && line < hi
}

func (a *App) activeLogWindow(path string) (string, int, int, int, int, bool) {
	p, cur, top, h, n, ok := a.EditorGroup.WindowView()
	if !ok || p != path {
		return "", 0, 0, 0, 0, false
	}
	return p, cur, top, h, n, true
}

func (a *App) logPaths() []string {
	var paths []string
	for path := range a.logViews {
		if !a.EditorGroup.HasWindowTab(path) {
			a.logViews[path].file.Close()
			delete(a.logViews, path)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func (a *App) syncLogStatus() {
	path, _, _, _, _, ok := a.EditorGroup.WindowView()
	lv := a.logViews[path]
	if !ok || lv == nil {
		a.Status.RemoveSegment("logmode")
		return
	}
	text := fmt.Sprintf("LOG %d lines", lv.window.Total)
	if lv.file.Indexing() {
		text += " (indexing…)"
	}
	a.Status.SetSegment(view.StatusSegment{ID: "logmode", Side: "left", Priority: 150, Text: text})
}
