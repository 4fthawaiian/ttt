package app

import (
	"fmt"

	"github.com/eugenioenko/ttt/internal/command"
	"github.com/eugenioenko/ttt/internal/ui"
	"github.com/eugenioenko/ttt/internal/widgets"
)

const workspaceEditorTitle = "Workspace Editor · drag across cells, then choose what goes there · Esc to finish"

// dockView is a single sidebar or panel tab that the workspace editor can lift
// out into its own block. Home is the tabbed host it returns to when cleared.
type dockView struct {
	ID     string
	Title  string
	Widget widgets.Widget
	Home   *ui.TabbedPanel
	Host   *ui.BottomPanelWidget
}

func (a *App) refreshDockViews() {
	if a.dockViews == nil {
		a.dockViews = map[string]*dockView{}
	}
	for _, home := range []*ui.TabbedPanel{&a.Sidebar.TabbedPanel, &a.BottomPanel.TabbedPanel} {
		for _, p := range home.PanelEntries() {
			if _, ok := a.dockViews[p.ID]; ok {
				continue
			}
			a.dockViews[p.ID] = &dockView{ID: p.ID, Title: p.Title, Widget: home.PanelWidget(p.ID), Home: home}
			a.dockViewOrder = append(a.dockViewOrder, p.ID)
		}
	}
}

func (a *App) ToggleWorkspaceEditor() {
	if a.MainGrid.Editing {
		a.exitWorkspaceEditor()
		return
	}
	if a.Root.HasOverlay() {
		return
	}
	a.refreshDockViews()
	a.MainGrid.Title = workspaceEditorTitle
	a.MainGrid.OnExitEditing = a.exitWorkspaceEditor
	a.MainGrid.OnPickCells = a.pickGridCells
	a.MainGrid.SetEditing(true)
	a.Root.SetFocus(a.MainGrid)
}

func (a *App) exitWorkspaceEditor() {
	a.MainGrid.SetEditing(false)
	a.MainGrid.Title = ""
	a.FocusEditor()
}

func (a *App) placedLabel(label string, w widgets.Widget) string {
	if _, ok := a.MainGrid.FindWidget(w); ok {
		return label + "  (placed)"
	}
	return label
}

func (a *App) workspaceItems(g *widgets.GridWidget) []widgets.SelectItem {
	items := []widgets.SelectItem{
		{ID: "editor", Label: a.placedLabel("Editor", a.EditorGroup)},
		{ID: "sidebar", Label: a.placedLabel("Sidebar (all sidebar views as tabs)", a.Sidebar)},
		{ID: "panel", Label: a.placedLabel("Bottom Panel (all panel views as tabs)", a.BottomPanel)},
	}
	for _, id := range a.dockViewOrder {
		v := a.dockViews[id]
		label := "View: " + v.Title
		if v.Host != nil {
			label += "  (placed)"
		}
		items = append(items, widgets.SelectItem{ID: "view:" + id, Label: label})
	}
	items = append(items,
		widgets.SelectItem{ID: "grid", Label: "Nested Grid (another 3x3 inside these cells)"},
		widgets.SelectItem{ID: "note", Label: "Placeholder Block"},
		widgets.SelectItem{ID: "clear", Label: "Clear These Cells"},
	)
	if g != a.MainGrid {
		items = append(items, widgets.SelectItem{ID: "remove-grid", Label: "Remove This Nested Grid"})
	}
	return items
}

func (a *App) pickGridCells(g *widgets.GridWidget, row, col, rows, cols int) {
	title := fmt.Sprintf("Place in %d×%d cells", rows, cols)
	a.ShowSelectDialog(title, a.workspaceItems(g), func(id string) {
		a.applyGridPick(g, id, row, col, rows, cols)
		g.ClearSelection()
		a.Root.SetFocus(a.MainGrid)
	}, nil)
}

func (a *App) applyGridPick(g *widgets.GridWidget, id string, row, col, rows, cols int) {
	area := widgets.GridArea{Row: row, Col: col, RowSpan: rows, ColSpan: cols}
	switch {
	case id == "clear":
		a.releaseGridAreas(g.ClearCells(row, col, rows, cols))
		return
	case id == "remove-grid":
		if owner, ok := a.MainGrid.FindWidget(g); ok {
			if removed, ok := owner.RemoveWidget(g); ok {
				a.releaseGridAreas([]widgets.GridArea{removed})
			}
		}
		return
	case id == "editor" || id == "sidebar" || id == "panel":
		area.Widget = a.gridPool[id]
		area.Name, area.Title = id, gridFixedTitles[id]
		a.detachGridWidget(area.Widget)
	case len(id) > 5 && id[:5] == "view:":
		v := a.dockViews[id[5:]]
		if v == nil {
			return
		}
		a.liftDockView(v)
		area.Widget, area.Name, area.Title = v.Host, "view-"+v.ID, v.Title
	case id == "grid":
		a.layoutSeq++
		area.Widget = a.newNestedGrid()
		area.Name, area.Title = fmt.Sprintf("grid%d", a.layoutSeq), "Nested Grid"
	case id == "note":
		a.layoutSeq++
		note := widgets.NewParagraphWidget("Placeholder block")
		note.SetBoxModel(widgets.BoxModel{PaddingLeft: 1})
		area.Widget = note
		area.Name, area.Title = fmt.Sprintf("note%d", a.layoutSeq), "Placeholder"
	default:
		return
	}
	removed, err := g.PlaceArea(area)
	if err != nil {
		a.StatusError("Workspace: " + err.Error())
		return
	}
	a.releaseGridAreas(removed)
}

// A widget lives in exactly one block, so placing it again moves it.
func (a *App) detachGridWidget(w widgets.Widget) {
	if owner, ok := a.MainGrid.FindWidget(w); ok {
		owner.RemoveWidget(w)
	}
}

func (a *App) liftDockView(v *dockView) {
	if v.Host != nil {
		a.detachGridWidget(v.Host)
		v.Host.RemovePanel(v.ID)
	} else {
		v.Home.RemovePanel(v.ID)
	}
	host := ui.NewBottomPanelWidget(a.Borders)
	host.AddPanel(v.ID, v.Title, v.Widget)
	host.SetActivePanel(v.ID)
	v.Host = host
}

func (a *App) releaseGridAreas(areas []widgets.GridArea) {
	for _, area := range areas {
		switch w := area.Widget.(type) {
		case *widgets.GridWidget:
			a.releaseGridAreas(w.Areas)
		case *ui.BottomPanelWidget:
			for _, v := range a.dockViews {
				if v.Host == w {
					v.Host.RemovePanel(v.ID)
					v.Home.AddPanel(v.ID, v.Title, v.Widget)
					v.Host = nil
				}
			}
		}
	}
}

func (a *App) newNestedGrid() *widgets.GridWidget {
	g := widgets.NewGridWidget(nil)
	g.BorderSet = a.Borders
	g.OnChildClick = a.onGridChildClick
	g.OnPickCells = a.pickGridCells
	g.SetEditing(a.MainGrid.Editing)
	return g
}

func registerWorkspaceEditorCommand(app *App) {
	app.Reg.Register(command.Command{
		ID: "layout.workspaceEditor", Title: "View: Workspace Editor",
		Keywords: []string{"grid", "layout", "workspace", "dock", "arrange", "panels"},
		Handler:  app.ToggleWorkspaceEditor,
	})
}
