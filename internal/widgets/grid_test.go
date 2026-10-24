package widgets

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func gridPool(names ...string) map[string]Widget {
	pool := map[string]Widget{}
	for _, n := range names {
		pool[n] = NewParagraphWidget(n)
	}
	return pool
}

func TestValidGridSpan(t *testing.T) {
	for r := 0; r <= 4; r++ {
		for c := 0; c <= 4; c++ {
			want := r >= 1 && r <= 3 && c >= 1 && c <= 3
			if got := ValidGridSpan(r, c); got != want {
				t.Errorf("ValidGridSpan(%d, %d) = %v", r, c, got)
			}
		}
	}
}

func TestParseGridTemplate(t *testing.T) {
	pool := gridPool("A", "B", "C", "D", "E")
	areas, err := ParseGridTemplate([]string{"A A B", "D E B", "D C C"}, pool)
	if err != nil {
		t.Fatal(err)
	}
	if got := GridTemplate(areas); got[0] != "A A B" || got[1] != "D E B" || got[2] != "D C C" {
		t.Fatalf("round trip = %v", got)
	}

	if _, err := ParseGridTemplate([]string{"E E A", "E E B", "C D ."}, pool); err != nil {
		t.Fatalf("2x2 span should be accepted: %v", err)
	}

	for _, bad := range [][]string{
		{"A A .", "A . .", ". . ."},
		{"A . A", ". . .", ". . ."},
		{"A B", ". . .", ". . ."},
		{"Z . .", ". . .", ". . ."},
	} {
		if _, err := ParseGridTemplate(bad, pool); err == nil {
			t.Errorf("template %v should be rejected", bad)
		}
	}
}

func TestGridCollapsesUnusedTracksAndSharesGutters(t *testing.T) {
	areas, _ := ParseGridTemplate([]string{". . .", "A B .", "A C ."}, gridPool("A", "B", "C"))
	g := NewGridWidget(areas)
	g.layout(41, 21)

	if g.colSize[2] != 0 || g.rowSize[0] != 0 {
		t.Fatalf("unused tracks should collapse: cols %v rows %v", g.colSize, g.rowSize)
	}
	if g.colSize[0]+g.colSize[1]+1 != 41 || g.rowSize[1]+g.rowSize[2]+1 != 21 {
		t.Fatalf("visible tracks plus one gutter should fill: cols %v rows %v", g.colSize, g.rowSize)
	}
	a := g.rects[0]
	if a.H != 21 {
		t.Fatalf("A spans both rows and the gutter between them, got height %d", a.H)
	}
}

func TestGridGutterDragResizesWholeColumn(t *testing.T) {
	areas, _ := ParseGridTemplate([]string{"A B .", "C D .", ". . ."}, gridPool("A", "B", "C", "D"))
	g := NewGridWidget(areas)
	g.SetRect(Rect{W: 41, H: 21})
	g.layout(41, 21)
	gx := g.colPos[0] + g.colSize[0]

	g.HandleEvent(tcell.NewEventMouse(gx, 3, tcell.Button1, tcell.ModNone))
	g.HandleEvent(tcell.NewEventMouse(10, 3, tcell.Button1, tcell.ModNone))
	g.HandleEvent(tcell.NewEventMouse(10, 3, tcell.ButtonNone, tcell.ModNone))
	g.layout(41, 21)

	if g.colSize[0] != 10 {
		t.Fatalf("column 0 should be 10 wide after drag, got %d", g.colSize[0])
	}
	if g.rects[2].W != 10 {
		t.Fatalf("C shares column 0 and should resize with A")
	}
}

func TestGridArrangeSwapsAreas(t *testing.T) {
	areas, _ := ParseGridTemplate([]string{"A A B", ". . B", ". . ."}, gridPool("A", "B"))
	g := NewGridWidget(areas)
	g.Arranging = true
	g.SetRect(Rect{W: 40, H: 20})
	g.layout(40, 20)

	g.HandleEvent(tcell.NewEventMouse(2, 2, tcell.Button1, tcell.ModNone))
	g.HandleEvent(tcell.NewEventMouse(38, 15, tcell.Button1, tcell.ModNone))
	g.HandleEvent(tcell.NewEventMouse(38, 15, tcell.ButtonNone, tcell.ModNone))

	if got := GridTemplate(g.Areas); got[0] != "B B A" || got[1] != ". . A" {
		t.Fatalf("swap = %v", got)
	}
}

func TestGridTrackCollapsesAndReopensFromFrame(t *testing.T) {
	areas, _ := ParseGridTemplate([]string{"S E .", "S E .", ". . ."}, gridPool("S", "E"))
	g := NewGridWidget(areas)
	g.Box = BoxModel{BorderTop: true, BorderBottom: true, BorderLeft: true, BorderRight: true}
	g.SetRect(Rect{W: 42, H: 20})
	g.layout(42, 20)
	drag := func(x0, x1 int) {
		g.HandleEvent(tcell.NewEventMouse(x0, 5, tcell.Button1, tcell.ModNone))
		g.HandleEvent(tcell.NewEventMouse(x1, 5, tcell.Button1, tcell.ModNone))
		g.HandleEvent(tcell.NewEventMouse(x1, 5, tcell.ButtonNone, tcell.ModNone))
		g.layout(42, 20)
	}

	drag(g.colPos[0]+g.colSize[0], 2)
	if g.colSize[0] != 0 || g.rects[1].X != 1 || g.rects[1].W != 40 {
		t.Fatalf("S should collapse and E fill the frame: cols %v, E %v", g.colSize, g.rects[1])
	}

	drag(0, 12)
	if g.colSize[0] != 11 {
		t.Fatalf("dragging the left frame should reopen S to 11, got %d", g.colSize[0])
	}
}

func TestGridPlaceAreaEvictsOverlapsAndOpensTracks(t *testing.T) {
	pool := gridPool("A", "B", "C", "N")
	areas, _ := ParseGridTemplate([]string{"A B .", "C C .", ". . ."}, pool)
	g := NewGridWidget(areas)
	g.Cols = [3]float64{1, 1, 0}

	removed, err := g.PlaceArea(GridArea{Name: "N", Widget: pool["N"], Row: 0, Col: 1, RowSpan: 2, ColSpan: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("B and C overlap the new 2x2 and should be evicted, got %d", len(removed))
	}
	if got := GridTemplate(g.Areas); got[0] != "A N N" || got[1] != ". N N" {
		t.Fatalf("layout = %v", got)
	}
	if g.Cols[2] <= 0 {
		t.Fatalf("placing into a zero-weight column should open it, cols %v", g.Cols)
	}
}
