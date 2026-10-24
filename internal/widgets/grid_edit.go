package widgets

import (
	"fmt"

	"github.com/eugenioenko/ttt/internal/term"
	"github.com/gdamore/tcell/v3"
)

// EdgeJoins lists absolute rows (Left, Right) and columns (Top, Bottom) where
// a child's own lines reach its edges, so the parent grid tees into them.
type EdgeJoins struct {
	Left, Right []int
	Top, Bottom []int
}

type EdgeJoinProvider interface {
	EdgeJoins() EdgeJoins
}

type gridSelection struct {
	active   bool
	dragging bool
	r0, c0   int
	r1, c1   int
}

func (s gridSelection) bounds() (r0, c0, r1, c1 int) {
	return min(s.r0, s.r1), min(s.c0, s.c1), max(s.r0, s.r1), max(s.c0, s.c1)
}

func (g *GridWidget) Focusable() bool         { return g.Editing }
func (g *GridWidget) SetFocused(focused bool) { g.focused = focused }
func (g *GridWidget) IsFocused() bool         { return g.focused }
func (g *GridWidget) EdgeJoins() EdgeJoins    { return g.edgeJoins }
func (g *GridWidget) ClearSelection()         { g.sel = gridSelection{} }

func (g *GridWidget) SetEditing(on bool) {
	g.InvalidatePointerInteraction()
	g.Editing = on
	g.sel = gridSelection{}
	for _, a := range g.Areas {
		if child, ok := a.Widget.(*GridWidget); ok {
			child.SetEditing(on)
		}
	}
}

// trackWeights gives collapsed tracks a usable size while editing, so every
// cell can be selected even when nothing occupies it yet.
func (g *GridWidget) trackWeights(axis gridAxis) [gridTracks]float64 {
	weights := g.Cols
	if axis == gridAxisRows {
		weights = g.Rows
	}
	if g.Editing {
		fillNewTracks(&weights, [gridTracks]bool{true, true, true})
	}
	return weights
}

func spansOverlap(a, b GridArea) bool {
	return a.Row < b.Row+b.RowSpan && b.Row < a.Row+a.RowSpan &&
		a.Col < b.Col+b.ColSpan && b.Col < a.Col+a.ColSpan
}

// PlaceArea puts a into the grid, evicting every area it overlaps. The evicted
// areas are returned so their widgets can be sent back where they belong.
func (g *GridWidget) PlaceArea(a GridArea) ([]GridArea, error) {
	var kept, removed []GridArea
	for _, o := range g.Areas {
		if spansOverlap(o, a) {
			removed = append(removed, o)
		} else {
			kept = append(kept, o)
		}
	}
	kept = append(kept, a)
	if err := ValidateGridAreas(kept); err != nil {
		return nil, err
	}
	g.InvalidatePointerInteraction()
	g.Areas = kept
	fillNewTracks(&g.Cols, g.used(gridAxisCols))
	fillNewTracks(&g.Rows, g.used(gridAxisRows))
	return removed, nil
}

func (g *GridWidget) ClearCells(row, col, rows, cols int) []GridArea {
	probe := GridArea{Row: row, Col: col, RowSpan: rows, ColSpan: cols}
	var kept, removed []GridArea
	for _, o := range g.Areas {
		if spansOverlap(o, probe) {
			removed = append(removed, o)
		} else {
			kept = append(kept, o)
		}
	}
	g.InvalidatePointerInteraction()
	g.Areas = kept
	return removed
}

func (g *GridWidget) RemoveWidget(w Widget) (GridArea, bool) {
	for i, a := range g.Areas {
		if a.Widget == w {
			g.InvalidatePointerInteraction()
			g.Areas = append(g.Areas[:i:i], g.Areas[i+1:]...)
			return a, true
		}
	}
	return GridArea{}, false
}

// FindWidget searches this grid and its nested grids for the grid holding w.
func (g *GridWidget) FindWidget(w Widget) (*GridWidget, bool) {
	for _, a := range g.Areas {
		if a.Widget == w {
			return g, true
		}
		if child, ok := a.Widget.(*GridWidget); ok {
			if owner, ok := child.FindWidget(w); ok {
				return owner, true
			}
		}
	}
	return nil, false
}

func (g *GridWidget) renderEditing(surface Surface, w, h int) {
	r := g.GetRect()
	for idx, a := range g.Areas {
		ar := g.rects[idx]
		if ar.W <= 0 || ar.H <= 0 {
			continue
		}
		if child, ok := a.Widget.(*GridWidget); ok {
			if !child.Editing {
				child.SetEditing(true)
			}
			child.SetRect(Rect{X: r.X + ar.X, Y: r.Y + ar.Y, W: ar.W, H: ar.H})
			child.Render(surface.Sub(ar))
			continue
		}
		InvalidatePointerInteraction(a.Widget)
		title := a.Title
		if title == "" {
			title = a.Name
		}
		mid := ar.Y + ar.H/2
		g.drawCentered(surface, Rect{X: ar.X, Y: mid, W: ar.W, H: 1}, title, term.StyleBorderActive)
		if ar.H > 2 {
			g.drawCentered(surface, Rect{X: ar.X, Y: mid + 1, W: ar.W, H: 1}, fmt.Sprintf("%d×%d", a.RowSpan, a.ColSpan), term.StyleBorder)
		}
	}
	for row := range gridTracks {
		for col := range gridTracks {
			if g.areaAtCell(row, col) < 0 {
				g.drawCentered(surface, g.cellRect(row, col), "+", term.StyleBorder)
			}
		}
	}
	if g.sel.active {
		r0, c0, r1, c1 := g.sel.bounds()
		sr := Rect{
			X: g.colPos[c0],
			Y: g.rowPos[r0],
			W: g.colPos[c1] + g.colSize[c1] - g.colPos[c0],
			H: g.rowPos[r1] + g.rowSize[r1] - g.rowPos[r0],
		}
		surface.ClearRect(sr.X, sr.Y, sr.W, sr.H, term.StyleSelection)
		g.drawCentered(surface, sr, fmt.Sprintf("%d×%d", r1-r0+1, c1-c0+1), term.StyleSelection)
	}
	g.renderLines(surface, w, h)
	if g.Title != "" && g.Box.BorderTop && w > 4 {
		surface.DrawText(2, 0, " "+g.Title+" ", w-2, term.StyleBorderActive)
	}
}

// A child's own edge line (a tab underline) reaches the edge inside the child,
// so it counts as running inward even though the next cell is content.
func (g *GridWidget) collectEdgeJoins(isLine func(x, y int) bool, joins map[[2]int]bool, w, h int) {
	r := g.GetRect()
	j := EdgeJoins{}
	for y := range h {
		if isLine(0, y) && (isLine(1, y) || joins[[2]int{0, y}]) {
			j.Left = append(j.Left, r.Y+y)
		}
		if isLine(w-1, y) && (isLine(w-2, y) || joins[[2]int{w - 1, y}]) {
			j.Right = append(j.Right, r.Y+y)
		}
	}
	for x := range w {
		if isLine(x, 0) && isLine(x, 1) {
			j.Top = append(j.Top, r.X+x)
		}
		if isLine(x, h-1) && isLine(x, h-2) {
			j.Bottom = append(j.Bottom, r.X+x)
		}
	}
	g.edgeJoins = j
}

func (g *GridWidget) handleEditingKey(kev *tcell.EventKey) EventResult {
	if !g.Editing {
		return EventIgnored
	}
	if kev.Key() == tcell.KeyEscape && g.OnExitEditing != nil {
		g.OnExitEditing()
		return EventConsumed
	}
	return EventIgnored
}

func (g *GridWidget) handleEditingMouse(ev tcell.Event, lx, ly int, fresh bool) EventResult {
	if idx := g.ownerAt(lx, ly); idx >= 0 {
		if child, ok := g.Areas[idx].Widget.(*GridWidget); ok {
			result := child.HandleEvent(ev)
			if result == EventCaptured {
				g.captured = child
				return result
			}
			return EventConsumed
		}
	}
	if !fresh {
		return EventConsumed
	}
	row, col := g.cellAt(lx, ly)
	if row < 0 || col < 0 {
		return EventConsumed
	}
	g.sel = gridSelection{active: true, dragging: true, r0: row, c0: col, r1: row, c1: col}
	return EventCaptured
}

// A single click selects the whole area under it; a drag selects exactly the
// cells it crosses, so an area can be grown, shrunk or replaced.
func (g *GridWidget) dragSelection(lx, ly int, pressed bool) EventResult {
	if row, col := g.cellAt(lx, ly); row >= 0 && col >= 0 {
		g.sel.r1, g.sel.c1 = row, col
	}
	if pressed {
		return EventCaptured
	}
	g.sel.dragging = false
	r0, c0, r1, c1 := g.sel.bounds()
	if r0 == r1 && c0 == c1 {
		if i := g.areaAtCell(r0, c0); i >= 0 {
			a := g.Areas[i]
			r0, c0, r1, c1 = a.Row, a.Col, a.Row+a.RowSpan-1, a.Col+a.ColSpan-1
		}
	}
	g.sel = gridSelection{active: true, r0: r0, c0: c0, r1: r1, c1: c1}
	if g.OnPickCells != nil {
		g.OnPickCells(g, r0, c0, r1-r0+1, c1-c0+1)
	}
	return EventConsumed
}
