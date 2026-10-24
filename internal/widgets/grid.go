package widgets

import (
	"fmt"
	"strings"

	"github.com/eugenioenko/ttt/internal/term"
	"github.com/eugenioenko/ttt/internal/textwidth"
	"github.com/gdamore/tcell/v3"
)

const gridTracks = 3

type GridArea struct {
	Name    string
	Title   string
	Widget  Widget
	Row     int
	Col     int
	RowSpan int
	ColSpan int
}

func (a GridArea) contains(row, col int) bool {
	return row >= a.Row && row < a.Row+a.RowSpan && col >= a.Col && col < a.Col+a.ColSpan
}

func ValidGridSpan(rows, cols int) bool {
	return rows >= 1 && rows <= gridTracks && cols >= 1 && cols <= gridTracks
}

func ValidateGridAreas(areas []GridArea) error {
	var owner [gridTracks][gridTracks]string
	for _, a := range areas {
		if a.Widget == nil {
			return fmt.Errorf("area %q has no widget", a.Name)
		}
		if !ValidGridSpan(a.RowSpan, a.ColSpan) {
			return fmt.Errorf("area %q: %dx%d is not an allowed span", a.Name, a.RowSpan, a.ColSpan)
		}
		if a.Row < 0 || a.Col < 0 || a.Row+a.RowSpan > gridTracks || a.Col+a.ColSpan > gridTracks {
			return fmt.Errorf("area %q is outside the grid", a.Name)
		}
		for r := a.Row; r < a.Row+a.RowSpan; r++ {
			for c := a.Col; c < a.Col+a.ColSpan; c++ {
				if owner[r][c] != "" {
					return fmt.Errorf("areas %q and %q overlap", owner[r][c], a.Name)
				}
				owner[r][c] = a.Name
			}
		}
	}
	return nil
}

// ParseGridTemplate reads three rows of three names ("." is an empty cell),
// like CSS grid-template-areas. Every name must form a rectangle.
func ParseGridTemplate(rows []string, widgets map[string]Widget) ([]GridArea, error) {
	if len(rows) != gridTracks {
		return nil, fmt.Errorf("template needs %d rows, got %d", gridTracks, len(rows))
	}
	var cells [gridTracks][gridTracks]string
	var order []string
	seen := map[string]bool{}
	for r, row := range rows {
		names := strings.Fields(row)
		if len(names) != gridTracks {
			return nil, fmt.Errorf("row %d needs %d cells, got %d", r+1, gridTracks, len(names))
		}
		for c, name := range names {
			if name == "." {
				continue
			}
			cells[r][c] = name
			if !seen[name] {
				seen[name] = true
				order = append(order, name)
			}
		}
	}
	areas := make([]GridArea, 0, len(order))
	for _, name := range order {
		minR, minC, maxR, maxC, count := gridTracks, gridTracks, -1, -1, 0
		for r := range gridTracks {
			for c := range gridTracks {
				if cells[r][c] != name {
					continue
				}
				minR, minC = min(minR, r), min(minC, c)
				maxR, maxC = max(maxR, r), max(maxC, c)
				count++
			}
		}
		rs, cs := maxR-minR+1, maxC-minC+1
		if count != rs*cs {
			return nil, fmt.Errorf("area %q is not a rectangle", name)
		}
		w, ok := widgets[name]
		if !ok {
			return nil, fmt.Errorf("unknown area %q", name)
		}
		areas = append(areas, GridArea{Name: name, Widget: w, Row: minR, Col: minC, RowSpan: rs, ColSpan: cs})
	}
	if err := ValidateGridAreas(areas); err != nil {
		return nil, err
	}
	return areas, nil
}

func GridTemplate(areas []GridArea) []string {
	var cells [gridTracks][gridTracks]string
	for r := range gridTracks {
		for c := range gridTracks {
			cells[r][c] = "."
		}
	}
	for _, a := range areas {
		for r := a.Row; r < a.Row+a.RowSpan; r++ {
			for c := a.Col; c < a.Col+a.ColSpan; c++ {
				cells[r][c] = a.Name
			}
		}
	}
	rows := make([]string, gridTracks)
	for r := range gridTracks {
		rows[r] = strings.Join(cells[r][:], " ")
	}
	return rows
}

// EdgeLineProvider reports absolute rows where a child draws a horizontal line
// that reaches its left and right edges, so the grid joins its dividers to it.
type EdgeLineProvider interface {
	EdgeLineRows() []int
}

// BottomEdgeProvider reports absolute columns where a child's vertical line
// reaches its bottom edge, so the grid draws a tee into the line below.
type BottomEdgeProvider interface {
	BottomEdgeCols() []int
}

// TopEdgeProvider is a child that draws its own top edge, like a tab bar whose
// underline closes the area. The grid draws nothing above the returned
// absolute row, so that line becomes the area's top border.
type TopEdgeProvider interface {
	TopEdgeRow() int
}

type gridAxis int

const (
	gridAxisCols gridAxis = iota
	gridAxisRows
)

type gridResize struct {
	active bool
	axis   gridAxis
	before int
	after  int
	// A collapsed track between two open ones reopens toward whichever side
	// the drag first moves, so the split is decided on the first motion.
	middle    int
	startV    int
	undecided bool
}

// GridWidget lays children out on a 3x3 grid of resizable tracks. Tracks no
// area touches collapse. Adjacent tracks are separated by a one-cell gutter
// that is both the drag handle and, when Dividers is set, the drawn border.
type GridWidget struct {
	BaseWidget
	Areas        []GridArea
	Cols         [gridTracks]float64
	Rows         [gridTracks]float64
	Dividers     bool
	BorderSet    *term.BorderSet
	MinTrack     int
	Arranging    bool
	OnChildClick func(w Widget)
	OnChange     func()

	Editing       bool
	Title         string
	OnPickCells   func(g *GridWidget, row, col, rows, cols int)
	OnExitEditing func()

	colPos, colSize [gridTracks]int
	rowPos, rowSize [gridTracks]int
	rects           []Rect
	owner           []int
	ownerW, ownerH  int
	resize          gridResize
	moving          int
	moveTargetRow   int
	moveTargetCol   int
	captured        Widget
	wasPressed      bool
	sel             gridSelection
	focused         bool
	edgeJoins       EdgeJoins
}

func NewGridWidget(areas []GridArea) *GridWidget {
	return &GridWidget{
		Areas:    areas,
		Cols:     [gridTracks]float64{1, 1, 1},
		Rows:     [gridTracks]float64{1, 1, 1},
		Dividers: true,
		MinTrack: 3,
		moving:   -1,
	}
}

func (g *GridWidget) Width() int  { return 0 }
func (g *GridWidget) Height() int { return 0 }

func (g *GridWidget) WidgetChildren() []Widget {
	children := make([]Widget, 0, len(g.Areas))
	for _, a := range g.Areas {
		children = append(children, a.Widget)
	}
	return children
}

func (g *GridWidget) SetAreas(areas []GridArea) error {
	if err := ValidateGridAreas(areas); err != nil {
		return err
	}
	g.InvalidatePointerInteraction()
	g.Areas = areas
	fillNewTracks(&g.Cols, g.used(gridAxisCols))
	fillNewTracks(&g.Rows, g.used(gridAxisRows))
	return nil
}

func fillNewTracks(weights *[gridTracks]float64, used [gridTracks]bool) {
	total, n := 0.0, 0
	for i := range gridTracks {
		if used[i] && weights[i] > 0 {
			total += weights[i]
			n++
		}
	}
	fill := 1.0
	if n > 0 {
		fill = total / float64(n)
	}
	for i := range gridTracks {
		if used[i] && weights[i] <= 0 {
			weights[i] = fill
		}
	}
}

func (g *GridWidget) AreaByName(name string) (GridArea, bool) {
	for _, a := range g.Areas {
		if a.Name == name {
			return a, true
		}
	}
	return GridArea{}, false
}

func (g *GridWidget) used(axis gridAxis) [gridTracks]bool {
	var used [gridTracks]bool
	for _, a := range g.Areas {
		start, span := a.Col, a.ColSpan
		if axis == gridAxisRows {
			start, span = a.Row, a.RowSpan
		}
		for i := start; i < start+span && i < gridTracks; i++ {
			used[i] = true
		}
	}
	return used
}

func layoutTracks(weights [gridTracks]float64, used [gridTracks]bool, start, length int) (pos, size [gridTracks]int) {
	visible := 0
	total := 0.0
	last := -1
	for i := range gridTracks {
		if !used[i] {
			continue
		}
		visible++
		last = i
		total += max(weights[i], 0.0001)
	}
	space := max(length-(visible-1), 0)
	if visible == 0 {
		for i := range gridTracks {
			pos[i] = start
		}
		return pos, size
	}
	assigned := 0
	for i := range gridTracks {
		if used[i] {
			size[i] = int(float64(space) * max(weights[i], 0.0001) / total)
			assigned += size[i]
		}
	}
	size[last] += space - assigned
	x := start
	for i := range gridTracks {
		pos[i] = x
		if used[i] {
			x += size[i]
			if i != last {
				x++
			}
		}
	}
	return pos, size
}

func (g *GridWidget) innerRect(w, h int) Rect {
	r := Rect{W: w, H: h}
	if g.Box.BorderLeft {
		r.X++
		r.W--
	}
	if g.Box.BorderRight {
		r.W--
	}
	if g.Box.BorderTop {
		r.Y++
		r.H--
	}
	if g.Box.BorderBottom {
		r.H--
	}
	r.W, r.H = max(r.W, 0), max(r.H, 0)
	return r
}

func (g *GridWidget) layout(w, h int) {
	inner := g.innerRect(w, h)
	g.colPos, g.colSize = layoutTracks(g.trackWeights(gridAxisCols), g.visible(gridAxisCols), inner.X, inner.W)
	g.rowPos, g.rowSize = layoutTracks(g.trackWeights(gridAxisRows), g.visible(gridAxisRows), inner.Y, inner.H)

	g.ownerW, g.ownerH = w, h
	if cap(g.owner) < w*h {
		g.owner = make([]int, w*h)
	}
	g.owner = g.owner[:w*h]
	for i := range g.owner {
		g.owner[i] = -1
	}
	g.rects = g.rects[:0]
	for _, a := range g.Areas {
		g.rects = append(g.rects, g.areaRect(a))
	}
	if !g.Editing {
		g.absorbTopLines()
	}
	for idx := range g.Areas {
		ar := g.rects[idx]
		for y := ar.Y; y < ar.Y+ar.H; y++ {
			for x := ar.X; x < ar.X+ar.W; x++ {
				g.owner[y*w+x] = idx
			}
		}
	}
}

// absorbTopLines grows an area that draws its own top edge into the line row
// above it when no other area borders that row, so no blank row is left where
// a border would have been.
func (g *GridWidget) absorbTopLines() {
	for idx, a := range g.Areas {
		if _, ok := a.Widget.(TopEdgeProvider); !ok {
			continue
		}
		ar := g.rects[idx]
		y := ar.Y - 1
		if y < 0 || ar.W <= 0 {
			continue
		}
		if g.rowBordered(idx, y, ar.X, ar.X+ar.W) {
			continue
		}
		g.rects[idx] = Rect{X: ar.X, Y: y, W: ar.W, H: ar.H + 1}
	}
}

func (g *GridWidget) rowBordered(self, y, x0, x1 int) bool {
	for idx, o := range g.rects {
		if idx == self || o.W <= 0 || o.H <= 0 {
			continue
		}
		if x1 <= o.X-1 || x0 > o.X+o.W {
			continue
		}
		if y == o.Y-1 || y == o.Y+o.H || (y >= o.Y && y < o.Y+o.H) {
			return true
		}
	}
	return false
}

func (g *GridWidget) cellRect(row, col int) Rect {
	return Rect{X: g.colPos[col], Y: g.rowPos[row], W: g.colSize[col], H: g.rowSize[row]}
}

func (g *GridWidget) areaRect(a GridArea) Rect {
	c1 := a.Col + a.ColSpan - 1
	r1 := a.Row + a.RowSpan - 1
	return Rect{
		X: g.colPos[a.Col],
		Y: g.rowPos[a.Row],
		W: g.colPos[c1] + g.colSize[c1] - g.colPos[a.Col],
		H: g.rowPos[r1] + g.rowSize[r1] - g.rowPos[a.Row],
	}
}

func (g *GridWidget) ownerAt(x, y int) int {
	if x < 0 || y < 0 || x >= g.ownerW || y >= g.ownerH {
		return -1
	}
	return g.owner[y*g.ownerW+x]
}

func (g *GridWidget) Render(surface Surface) {
	w, h := surface.Size()
	if w <= 0 || h <= 0 {
		g.InvalidatePointerInteraction()
		return
	}
	surface.ClearRect(0, 0, w, h, term.StyleDefault)
	g.layout(w, h)
	if g.Editing {
		g.renderEditing(surface, w, h)
		return
	}

	r := g.GetRect()
	for idx, a := range g.Areas {
		ar := g.rects[idx]
		if ar.W <= 0 || ar.H <= 0 {
			InvalidatePointerInteraction(a.Widget)
			continue
		}
		a.Widget.SetRect(Rect{X: r.X + ar.X, Y: r.Y + ar.Y, W: ar.W, H: ar.H})
		a.Widget.Render(surface.Sub(ar))
	}
	g.renderLines(surface, w, h)
	if g.Arranging || g.moving >= 0 {
		g.renderArrange(surface)
	}
}

func (g *GridWidget) lineCells(w, h int) []bool {
	lines := make([]bool, w*h)
	mark := func(x, y int) {
		if x >= 0 && y >= 0 && x < w && y < h {
			lines[y*w+x] = true
		}
	}
	for x := range w {
		if g.Box.BorderTop {
			mark(x, 0)
		}
		if g.Box.BorderBottom {
			mark(x, h-1)
		}
	}
	for y := range h {
		if g.Box.BorderLeft {
			mark(0, y)
		}
		if g.Box.BorderRight {
			mark(w-1, y)
		}
	}
	if !g.Dividers {
		return lines
	}
	inner := g.innerRect(w, h)
	for _, x := range g.gutters(gridAxisCols) {
		for y := inner.Y; y < inner.Y+inner.H; y++ {
			if g.ownerAt(x, y) < 0 {
				mark(x, y)
			}
		}
	}
	for _, y := range g.gutters(gridAxisRows) {
		for x := inner.X; x < inner.X+inner.W; x++ {
			if g.ownerAt(x, y) < 0 {
				mark(x, y)
			}
		}
	}
	return lines
}

func (g *GridWidget) topEdge(a GridArea, ar Rect) (int, bool) {
	p, ok := a.Widget.(TopEdgeProvider)
	if !ok {
		return 0, false
	}
	top := p.TopEdgeRow() - g.GetRect().Y
	if top < ar.Y || top >= ar.Y+ar.H {
		return 0, false
	}
	return top, true
}

// neededLines keeps only the line cells that border some area, so a corner
// whose neighbor needs no line resolves to a plain corner rather than a tee.
func (g *GridWidget) neededLines(lines []bool, w, h int) []bool {
	needed := make([]bool, w*h)
	need := func(x, y int) {
		if x >= 0 && y >= 0 && x < w && y < h {
			needed[y*w+x] = true
		}
	}
	for idx, a := range g.Areas {
		ar := g.rects[idx]
		if ar.W <= 0 || ar.H <= 0 {
			continue
		}
		top := ar.Y - 1
		if t, ok := g.topEdge(a, ar); ok {
			top = t
		} else {
			for x := ar.X - 1; x <= ar.X+ar.W; x++ {
				need(x, top)
			}
		}
		for y := top; y <= ar.Y+ar.H; y++ {
			need(ar.X-1, y)
			need(ar.X+ar.W, y)
		}
		for x := ar.X - 1; x <= ar.X+ar.W; x++ {
			need(x, ar.Y+ar.H)
		}
	}
	for i := range lines {
		lines[i] = lines[i] && needed[i]
	}
	return lines
}

func (g *GridWidget) gutters(axis gridAxis) []int {
	pos, size := g.colPos, g.colSize
	if axis == gridAxisRows {
		pos, size = g.rowPos, g.rowSize
	}
	used := g.visible(axis)
	var out []int
	for _, b := range gutterBoundaries(used) {
		out = append(out, pos[b[0]]+size[b[0]])
	}
	return out
}

func gutterBoundaries(used [gridTracks]bool) [][2]int {
	var out [][2]int
	prev := -1
	for i := range gridTracks {
		if !used[i] {
			continue
		}
		if prev >= 0 {
			out = append(out, [2]int{prev, i})
		}
		prev = i
	}
	return out
}

func (g *GridWidget) renderLines(surface Surface, w, h int) {
	lines := g.lineCells(w, h)
	if !g.Editing {
		lines = g.neededLines(lines, w, h)
	}
	joins := map[[2]int]bool{}
	r := g.GetRect()
	for idx, a := range g.Areas {
		ar := g.rects[idx]
		if ar.W <= 0 || ar.H <= 0 {
			continue
		}
		if p, ok := a.Widget.(EdgeJoinProvider); ok {
			j := p.EdgeJoins()
			for _, ay := range j.Left {
				joins[[2]int{ar.X, ay - r.Y}] = true
			}
			for _, ay := range j.Right {
				joins[[2]int{ar.X + ar.W - 1, ay - r.Y}] = true
			}
			for _, ax := range j.Top {
				joins[[2]int{ax - r.X, ar.Y}] = true
			}
			for _, ax := range j.Bottom {
				joins[[2]int{ax - r.X, ar.Y + ar.H - 1}] = true
			}
		}
		if g.Editing {
			continue
		}
		var rows []int
		if p, ok := a.Widget.(EdgeLineProvider); ok {
			rows = p.EdgeLineRows()
		}
		if top, ok := g.topEdge(a, ar); ok {
			rows = append(rows, r.Y+top)
		}
		for _, ay := range rows {
			y := ay - r.Y
			joins[[2]int{ar.X, y}] = true
			joins[[2]int{ar.X + ar.W - 1, y}] = true
		}
		if p, ok := a.Widget.(BottomEdgeProvider); ok {
			for _, ax := range p.BottomEdgeCols() {
				joins[[2]int{ax - r.X, ar.Y + ar.H - 1}] = true
			}
		}
	}
	isLine := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < w && y < h && (lines[y*w+x] || joins[[2]int{x, y}])
	}
	b := g.Box.Borders
	if g.BorderSet != nil {
		b = *g.BorderSet
	}
	if b.Vertical == 0 {
		b = term.SingleBorderSet()
	}
	style := g.Box.Style
	if style == 0 {
		style = term.StyleBorder
	}
	focused := Rect{X: -2, Y: -2}
	for idx, a := range g.Areas {
		if hasFocusedChild(a.Widget) {
			ar := g.rects[idx]
			focused = Rect{X: ar.X - 1, Y: ar.Y - 1, W: ar.W + 2, H: ar.H + 2}
		}
	}
	for y := range h {
		for x := range w {
			if !lines[y*w+x] {
				continue
			}
			ch := junctionRune(b, isLine(x, y-1), isLine(x, y+1), isLine(x-1, y), isLine(x+1, y))
			s := style
			if g.resize.active && g.onDraggedGutter(x, y) {
				s = term.StyleBorderActive
			} else if x >= focused.X && x < focused.X+focused.W && y >= focused.Y && y < focused.Y+focused.H {
				s = term.StyleBorderActive
			}
			surface.SetCell(x, y, term.Cell{Ch: ch, Style: s})
		}
	}
	g.collectEdgeJoins(isLine, joins, w, h)
}

func junctionRune(b term.BorderSet, up, down, left, right bool) rune {
	switch {
	case up && down && left && right:
		return b.Cross
	case left && right && down:
		return b.TopTee
	case left && right && up:
		return b.BottomTee
	case up && down && right:
		return b.LeftTee
	case up && down && left:
		return b.RightTee
	case down && right:
		return b.TopLeft
	case down && left:
		return b.TopRight
	case up && right:
		return b.BottomLeft
	case up && left:
		return b.BottomRight
	case left || right:
		return b.Horizontal
	}
	return b.Vertical
}

func (g *GridWidget) onDraggedGutter(x, y int) bool {
	if g.resize.axis == gridAxisCols {
		return x == g.colPos[g.resize.before]+g.colSize[g.resize.before]
	}
	return y == g.rowPos[g.resize.before]+g.rowSize[g.resize.before]
}

func (g *GridWidget) renderArrange(surface Surface) {
	if g.moving >= 0 && g.moveTargetRow >= 0 {
		if idx := g.areaAtCell(g.moveTargetRow, g.moveTargetCol); idx != g.moving {
			tr := g.cellRect(g.moveTargetRow, g.moveTargetCol)
			if idx >= 0 {
				tr = g.rects[idx]
			}
			surface.ClearRect(tr.X, tr.Y, tr.W, tr.H, term.StyleSelection)
		}
	}
	for idx, a := range g.Areas {
		label := " " + a.Name + " "
		if idx == g.moving {
			label = " moving " + a.Name + " "
		}
		g.drawCentered(surface, g.rects[idx], label, term.StyleSelection)
	}
	used := [2][gridTracks]bool{g.visible(gridAxisRows), g.visible(gridAxisCols)}
	for r := range gridTracks {
		for c := range gridTracks {
			if used[0][r] && used[1][c] && g.areaAtCell(r, c) < 0 {
				g.drawCentered(surface, g.cellRect(r, c), "empty", term.StyleBorder)
			}
		}
	}
}

func (g *GridWidget) drawCentered(surface Surface, r Rect, text string, style term.Style) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	x := r.X + max((r.W-textwidth.String(text))/2, 0)
	surface.DrawText(x, r.Y+r.H/2, text, r.X+r.W, style)
}

func (g *GridWidget) areaAtCell(row, col int) int {
	for idx, a := range g.Areas {
		if a.contains(row, col) {
			return idx
		}
	}
	return -1
}

func trackAt(pos, size [gridTracks]int, used [gridTracks]bool, v int) int {
	best := -1
	for i := range gridTracks {
		if !used[i] {
			continue
		}
		if v >= pos[i] {
			best = i
		}
		if v < pos[i]+size[i] {
			return best
		}
	}
	return best
}

func (g *GridWidget) visible(axis gridAxis) [gridTracks]bool {
	if g.Editing {
		return [gridTracks]bool{true, true, true}
	}
	used := g.used(axis)
	weights := g.Cols
	if axis == gridAxisRows {
		weights = g.Rows
	}
	for i := range gridTracks {
		used[i] = used[i] && weights[i] > 0
	}
	return used
}

// collapsedHandleAt finds the line a collapsed track sits on: the frame (or
// first/last cell when there is no frame) at an edge, or the gutter between
// two open tracks. Dragging it reopens the track.
func (g *GridWidget) collapsedHandleAt(axis gridAxis, v, start, length int) (gridResize, bool) {
	used, open := g.used(axis), g.visible(axis)
	pos := g.colPos
	if axis == gridAxisRows {
		pos = g.rowPos
	}
	end := start + length
	for c := range gridTracks {
		if !used[c] || open[c] {
			continue
		}
		prev, next := -1, -1
		for i := c - 1; i >= 0; i-- {
			if open[i] {
				prev = i
				break
			}
		}
		for i := c + 1; i < gridTracks; i++ {
			if open[i] {
				next = i
				break
			}
		}
		switch {
		case prev < 0 && next >= 0:
			handle := start - 1
			if !g.frameBefore(axis) {
				handle = start
			}
			if v == handle {
				return gridResize{active: true, axis: axis, before: c, after: next}, true
			}
		case next < 0 && prev >= 0:
			handle := pos[c]
			if !g.frameAfter(axis) {
				handle = end - 1
			}
			if v == handle {
				return gridResize{active: true, axis: axis, before: prev, after: c}, true
			}
		case prev >= 0 && next >= 0:
			if v == pos[c]-1 {
				return gridResize{active: true, axis: axis, before: prev, after: next, middle: c, startV: v, undecided: true}, true
			}
		}
	}
	return gridResize{}, false
}

func (g *GridWidget) frameBefore(axis gridAxis) bool {
	if axis == gridAxisRows {
		return g.Box.BorderTop
	}
	return g.Box.BorderLeft
}

func (g *GridWidget) frameAfter(axis gridAxis) bool {
	if axis == gridAxisRows {
		return g.Box.BorderBottom
	}
	return g.Box.BorderRight
}

func (g *GridWidget) cellAt(lx, ly int) (int, int) {
	col := trackAt(g.colPos, g.colSize, g.visible(gridAxisCols), lx)
	row := trackAt(g.rowPos, g.rowSize, g.visible(gridAxisRows), ly)
	return row, col
}

func (g *GridWidget) gutterAt(lx, ly int) (gridResize, bool) {
	inner := g.innerRect(g.ownerW, g.ownerH)
	if ly >= inner.Y && ly < inner.Y+inner.H {
		if rs, ok := g.collapsedHandleAt(gridAxisCols, lx, inner.X, inner.W); ok {
			return rs, true
		}
	}
	if lx >= inner.X && lx < inner.X+inner.W {
		if rs, ok := g.collapsedHandleAt(gridAxisRows, ly, inner.Y, inner.H); ok {
			return rs, true
		}
	}
	if g.ownerAt(lx, ly) >= 0 {
		return gridResize{}, false
	}
	if ly >= inner.Y && ly < inner.Y+inner.H {
		for _, b := range gutterBoundaries(g.visible(gridAxisCols)) {
			if lx == g.colPos[b[0]]+g.colSize[b[0]] {
				return gridResize{active: true, axis: gridAxisCols, before: b[0], after: b[1]}, true
			}
		}
	}
	if lx >= inner.X && lx < inner.X+inner.W {
		for _, b := range gutterBoundaries(g.visible(gridAxisRows)) {
			if ly == g.rowPos[b[0]]+g.rowSize[b[0]] {
				return gridResize{active: true, axis: gridAxisRows, before: b[0], after: b[1]}, true
			}
		}
	}
	return gridResize{}, false
}

func (g *GridWidget) dragResize(lx, ly int) {
	pos, size, weights := &g.colPos, &g.colSize, &g.Cols
	v := lx
	if g.resize.axis == gridAxisRows {
		pos, size, weights = &g.rowPos, &g.rowSize, &g.Rows
		v = ly
	}
	if g.resize.undecided {
		if v == g.resize.startV {
			return
		}
		g.resize.undecided = false
		if v > g.resize.startV {
			g.resize.before = g.resize.middle
		} else {
			g.resize.after = g.resize.middle
		}
	}
	b, a := g.resize.before, g.resize.after
	combined := size[b] + size[a]
	minTrack := max(g.MinTrack, 1)
	next := min(max(v-pos[b], 0), combined)
	if next < minTrack {
		next = 0
	} else if combined-next < minTrack {
		next = combined
	}
	wasOpen := size[b] > 0 && size[a] > 0
	nowOpen := next > 0 && next < combined
	switch {
	case nowOpen && !wasOpen:
		combined--
	case wasOpen && !nowOpen:
		combined++
	}
	next = min(next, combined)
	size[b], size[a] = next, combined-next
	used := g.used(g.resize.axis)
	for i := range gridTracks {
		if used[i] || g.Editing {
			weights[i] = float64(size[i])
		}
	}
}

func (g *GridWidget) finishMove(lx, ly int) {
	from := g.moving
	g.moving = -1
	g.moveTargetRow, g.moveTargetCol = -1, -1
	row, col := g.cellAt(lx, ly)
	if from < 0 || from >= len(g.Areas) || row < 0 || col < 0 {
		return
	}
	areas := append([]GridArea(nil), g.Areas...)
	src := areas[from]
	if to := g.areaAtCell(row, col); to >= 0 {
		if to == from {
			return
		}
		dst := areas[to]
		areas[from].Row, areas[from].Col, areas[from].RowSpan, areas[from].ColSpan = dst.Row, dst.Col, dst.RowSpan, dst.ColSpan
		areas[to].Row, areas[to].Col, areas[to].RowSpan, areas[to].ColSpan = src.Row, src.Col, src.RowSpan, src.ColSpan
	} else {
		areas[from].Row, areas[from].Col = row, col
		if ValidateGridAreas(areas) != nil {
			areas[from].RowSpan, areas[from].ColSpan = 1, 1
		}
	}
	if ValidateGridAreas(areas) != nil {
		return
	}
	g.Areas = areas
	if g.OnChange != nil {
		g.OnChange()
	}
}

func (g *GridWidget) HandleEvent(ev tcell.Event) EventResult {
	if kev, ok := ev.(*tcell.EventKey); ok {
		return g.handleEditingKey(kev)
	}
	mev, ok := ev.(*tcell.EventMouse)
	if !ok {
		return EventIgnored
	}
	r := g.GetRect()
	mx, my := mev.Position()
	lx, ly := mx-r.X, my-r.Y
	btn := mev.Buttons()
	pressed := btn&tcell.Button1 != 0
	fresh := pressed && !g.wasPressed
	g.wasPressed = pressed

	if g.resize.active {
		if pressed {
			g.dragResize(lx, ly)
			return EventCaptured
		}
		g.resize = gridResize{}
		if g.OnChange != nil {
			g.OnChange()
		}
		return EventConsumed
	}
	if g.sel.dragging {
		return g.dragSelection(lx, ly, pressed)
	}
	if g.moving >= 0 {
		if pressed {
			g.moveTargetRow, g.moveTargetCol = g.cellAt(lx, ly)
			return EventCaptured
		}
		g.finishMove(lx, ly)
		return EventConsumed
	}
	if g.captured != nil {
		child := g.captured
		if btn == tcell.ButtonNone {
			child.HandleEvent(ev)
			g.captured = nil
			return EventConsumed
		}
		if child.HandleEvent(ev) == EventCaptured {
			return EventCaptured
		}
		return EventConsumed
	}
	if lx < 0 || ly < 0 || lx >= r.W || ly >= r.H {
		return EventIgnored
	}
	if fresh {
		if rs, ok := g.gutterAt(lx, ly); ok {
			g.resize = rs
			return EventCaptured
		}
	}
	if g.Editing {
		return g.handleEditingMouse(ev, lx, ly, fresh)
	}
	idx := g.ownerAt(lx, ly)
	if idx < 0 {
		return EventConsumed
	}
	if fresh && (g.Arranging || mev.Modifiers()&tcell.ModAlt != 0) {
		g.moving = idx
		g.moveTargetRow, g.moveTargetCol = g.cellAt(lx, ly)
		return EventCaptured
	}
	if g.Arranging {
		return EventConsumed
	}
	child := g.Areas[idx].Widget
	result := child.HandleEvent(ev)
	if result == EventCaptured {
		g.captured = child
	}
	if fresh && g.OnChildClick != nil {
		g.OnChildClick(child)
	}
	return result
}

func (g *GridWidget) PointerShape(mx, my int) string {
	r := g.GetRect()
	lx, ly := mx-r.X, my-r.Y
	rs := g.resize
	if !rs.active {
		var ok bool
		if rs, ok = g.gutterAt(lx, ly); !ok {
			if g.moving >= 0 {
				return "grabbing"
			}
			return ""
		}
	}
	if rs.axis == gridAxisCols {
		return "ew-resize"
	}
	return "ns-resize"
}

func (g *GridWidget) Dragging() bool { return g.resize.active || g.moving >= 0 }

func (g *GridWidget) CancelPointerCapture() bool {
	canceled := g.resize.active || g.moving >= 0 || g.captured != nil
	g.resize = gridResize{}
	g.moving = -1
	g.captured = nil
	g.wasPressed = false
	for _, a := range g.Areas {
		canceled = CancelPointerCapture(a.Widget) || canceled
	}
	return canceled
}

func (g *GridWidget) InvalidatePointerInteraction() bool {
	invalidated := g.resize.active || g.moving >= 0 || g.captured != nil || g.sel.dragging
	g.sel.dragging = false
	g.resize = gridResize{}
	g.moving = -1
	g.captured = nil
	g.wasPressed = false
	for _, a := range g.Areas {
		invalidated = InvalidatePointerInteraction(a.Widget) || invalidated
	}
	return invalidated
}

func (g *GridWidget) OwnsPointerCapture() bool {
	if g.resize.active || g.moving >= 0 || g.sel.dragging {
		return true
	}
	if g.captured != nil {
		owner, ok := g.captured.(PointerCaptureOwner)
		if !ok || owner.OwnsPointerCapture() {
			return true
		}
		g.captured = nil
		g.wasPressed = false
	}
	return false
}
