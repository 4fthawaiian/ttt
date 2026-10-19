package ui

import (
	"slices"
	"testing"

	"github.com/eugenioenko/ttt/internal/term"
)

func TestVerticalTabBarShowsALabelBadge(t *testing.T) {
	v := NewVerticalTabBar()
	v.Count = 2
	v.Active = 0
	v.Labels = []string{"pi", ""}
	v.SetRect(Rect{X: 0, Y: 0, W: VerticalTabBarWidth, H: 4})

	grid := makeGrid(VerticalTabBarWidth, 4)
	surface := NewRenderSurface(grid, Rect{X: 0, Y: 0, W: VerticalTabBarWidth, H: 4})
	v.Render(surface)

	// row 0: the labelled tab shows [pi] instead of the number
	for x, want := range "[pi]" {
		if grid[0][x].Ch != want {
			t.Fatalf("row 0 col %d = %q, want %q", x, grid[0][x].Ch, want)
		}
	}
	if grid[0][0].Style != term.StyleActiveTab {
		t.Fatal("the active labelled tab must keep the active style")
	}
	// row 1: an unlabelled tab keeps the numbered badge
	for x, want := range "[>2]" {
		if grid[1][x].Ch != want {
			t.Fatalf("row 1 col %d = %q, want %q", x, grid[1][x].Ch, want)
		}
	}
}

func TestVerticalTabBarTruncatesLongLabels(t *testing.T) {
	v := NewVerticalTabBar()
	v.Count = 1
	v.Labels = []string{"planner"}
	v.SetRect(Rect{X: 0, Y: 0, W: VerticalTabBarWidth, H: 2})

	grid := makeGrid(VerticalTabBarWidth, 2)
	surface := NewRenderSurface(grid, Rect{X: 0, Y: 0, W: VerticalTabBarWidth, H: 2})
	v.Render(surface)

	// the strip is 4 content cells: a long label is clipped, never spills over
	// the border at x=4
	for x, want := range "[pl]" {
		if grid[0][x].Ch != want {
			t.Fatalf("col %d = %q, want %q", x, grid[0][x].Ch, want)
		}
	}
}

func TestTerminalPanelKeepsLabelsAlignedWithTabs(t *testing.T) {
	tp := NewTerminalPanelWidget()
	tp.AddTerminalLabeled(&captureStubWidget{}, "pi")
	tp.AddTerminal(&captureStubWidget{})
	tp.AddTerminalLabeled(&captureStubWidget{}, "lg")

	if got := tp.TabBar.Labels; !slices.Equal(got, []string{"pi", "", "lg"}) {
		t.Fatalf("labels = %v, want [pi  lg]", got)
	}

	tp.RemoveTerminal(0)
	if got := tp.TabBar.Labels; !slices.Equal(got, []string{"", "lg"}) {
		t.Fatalf("labels after remove = %v, want [ lg]", got)
	}
	if tp.TabBar.Count != 2 {
		t.Fatalf("count = %d, want 2", tp.TabBar.Count)
	}
}
