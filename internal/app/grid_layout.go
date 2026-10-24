package app

import (
	"strings"

	"github.com/eugenioenko/ttt/internal/command"
	"github.com/eugenioenko/ttt/internal/term"
	"github.com/eugenioenko/ttt/internal/ui"
	"github.com/eugenioenko/ttt/internal/widgets"
)

var defaultGridTemplate = []string{"sidebar editor .", "sidebar editor .", "sidebar panel ."}

var (
	defaultGridCols = [3]float64{20, 80, 0}
	defaultGridRows = [3]float64{35, 35, 30}
)

var gridFixedTitles = map[string]string{"editor": "Editor", "sidebar": "Sidebar", "panel": "Bottom Panel"}

func parseGridTemplate(rows []string, pool map[string]widgets.Widget) ([]widgets.GridArea, error) {
	areas, err := widgets.ParseGridTemplate(rows, pool)
	for i := range areas {
		areas[i].Title = gridFixedTitles[areas[i].Name]
	}
	return areas, err
}

func buildMainGrid(editor, sidebar, panel widgets.Widget, borders *term.BorderSet) (*widgets.GridWidget, map[string]widgets.Widget) {
	pool := map[string]widgets.Widget{"editor": editor, "sidebar": sidebar, "panel": panel}
	areas, err := parseGridTemplate(defaultGridTemplate, pool)
	if err != nil {
		panic(err)
	}
	grid := widgets.NewGridWidget(areas)
	grid.Cols = defaultGridCols
	grid.Rows = defaultGridRows
	grid.Box = widgets.BoxModel{BorderTop: true, BorderBottom: true, BorderLeft: true, BorderRight: true}
	grid.BorderSet = borders
	return grid, pool
}

func (a *App) GridToggleArrange() {
	a.MainGrid.Arranging = !a.MainGrid.Arranging
	if a.MainGrid.Arranging {
		a.StatusNotify("Arrange mode: drag a block onto another to swap, or onto an empty cell to move it")
	}
}

func (a *App) GridToggleDividers() {
	a.MainGrid.Dividers = !a.MainGrid.Dividers
}

func (a *App) GridToggleFrame() {
	b := &a.MainGrid.Box
	on := !b.BorderTop
	b.BorderTop, b.BorderBottom, b.BorderLeft, b.BorderRight = on, on, on, on
}

func (a *App) GridEditTemplate() {
	current := strings.Join(widgets.GridTemplate(a.MainGrid.Areas), " / ")
	a.ShowInputDialogEx("Grid Template (rows separated by /)", "sidebar editor . / sidebar editor . / sidebar panel .", current, "Apply", func(text string) {
		rows := strings.Split(text, "/")
		areas, err := parseGridTemplate(rows, a.gridPool)
		if err != nil {
			a.StatusError("Grid template: " + err.Error())
			return
		}
		if err := a.MainGrid.SetAreas(areas); err != nil {
			a.StatusError("Grid template: " + err.Error())
		}
	})
}

func (a *App) GridReset() {
	areas, err := parseGridTemplate(defaultGridTemplate, a.gridPool)
	if err != nil {
		a.StatusError("Grid template: " + err.Error())
		return
	}
	_ = a.MainGrid.SetAreas(areas)
	a.MainGrid.Cols = defaultGridCols
	a.MainGrid.Rows = defaultGridRows
}

func (a *App) onGridChildClick(w widgets.Widget) {
	if a.Root.HasOverlay() {
		return
	}
	switch host := w.(type) {
	case *ui.EditorGroupWidget:
		a.Root.SetFocus(host)
	case *ui.SidebarWidget:
		a.Reg.Execute("sidebar.focus")
	case *ui.BottomPanelWidget:
		if host.ActivePanel == "terminal" && len(a.Terminals) == 0 {
			a.SpawnTerminal()
			return
		}
		if active := host.ActiveWidget(); active != nil {
			a.Root.SetFocus(active)
		}
	}
}

// The old layout opened the panel on demand; the grid always shows it, so it
// needs a terminal from the start. Spawning needs the screen to post wakeups.
func (a *App) startGridTerminal() {
	if a.MainGrid == nil || len(a.Terminals) > 0 {
		return
	}
	a.ContentSplit.ShowBottom = true
	a.SpawnTerminal()
	a.Root.SetFocus(a.EditorGroup)
}

func (a *App) gridContains(w widgets.Widget, mx, my int) bool {
	if w == nil {
		return false
	}
	r := w.GetRect()
	return mx >= r.X && mx < r.X+r.W && my >= r.Y && my < r.Y+r.H
}

func registerGridCommands(app *App) {
	registerWorkspaceEditorCommand(app)
	reg := app.Reg
	reg.Register(command.Command{
		ID: "layout.toggleArrange", Title: "Layout: Toggle Arrange Mode",
		Keywords: []string{"grid", "layout", "move", "swap", "blocks"},
		Handler:  app.GridToggleArrange,
	})
	reg.Register(command.Command{
		ID: "layout.editTemplate", Title: "Layout: Edit Grid Template",
		Keywords: []string{"grid", "layout", "span", "areas"},
		Handler:  app.GridEditTemplate,
	})
	reg.Register(command.Command{
		ID: "layout.toggleDividers", Title: "Layout: Toggle Grid Dividers",
		Keywords: []string{"grid", "layout", "borders", "lines"},
		Handler:  app.GridToggleDividers,
	})
	reg.Register(command.Command{
		ID: "layout.toggleFrame", Title: "Layout: Toggle Grid Frame",
		Keywords: []string{"grid", "layout", "borders", "outer"},
		Handler:  app.GridToggleFrame,
	})
	reg.Register(command.Command{
		ID: "layout.reset", Title: "Layout: Reset Grid",
		Keywords: []string{"grid", "layout", "default"},
		Handler:  app.GridReset,
	})
}
