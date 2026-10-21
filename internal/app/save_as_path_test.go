package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eugenioenko/ttt/internal/workspace"
)

func TestSaveAsPathResolvesAgainstPrimaryFolder(t *testing.T) {
	primary := t.TempDir()
	second := t.TempDir()
	a := &App{Workspace: workspace.New([]string{primary, second})}

	tests := map[string]string{
		"note.txt":                       filepath.Join(primary, "note.txt"),
		"sub/note.txt":                   filepath.Join(primary, "sub", "note.txt"),
		"../sibling.txt":                 filepath.Join(filepath.Dir(primary), "sibling.txt"),
		filepath.Join(second, "abs.txt"): filepath.Join(second, "abs.txt"),
	}
	for input, want := range tests {
		if got := a.saveAsPath(input); got != want {
			t.Errorf("saveAsPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSaveAsPathExpandsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	a := &App{Workspace: workspace.New([]string{t.TempDir()})}
	if got, want := a.saveAsPath("~/note.txt"), filepath.Join(home, "note.txt"); got != want {
		t.Errorf("saveAsPath(~/note.txt) = %q, want %q", got, want)
	}
}

func TestSaveAsPathWithoutFolderKeepsRelativeName(t *testing.T) {
	for _, a := range []*App{{}, {Workspace: &workspace.Workspace{}}} {
		if got := a.saveAsPath("note.txt"); got != "note.txt" {
			t.Errorf("saveAsPath(note.txt) without a folder = %q, want it unchanged", got)
		}
	}
}
