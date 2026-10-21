package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// Replace All re-scans every listed file right before writing, so a file
// edited since the search (outside ttt, or by an earlier replacement in the
// same batch) is not replaced using offsets that no longer exist.
func TestReplaceAllFilesLeavesOutAFileTheRescanNoLongerMatches(t *testing.T) {
	requireRipgrep(t)
	dir := t.TempDir()
	stillMatches := filepath.Join(dir, "still.txt")
	noLongerMatches := filepath.Join(dir, "gone.txt")
	if err := os.WriteFile(stillMatches, []byte("foo here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// On disk this file no longer contains "foo" - as if it was already
	// fixed - but the panel still lists the stale match from the original
	// search.
	if err := os.WriteFile(noLongerMatches, []byte("nothing to see\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSearchWidget()
	s.Input.Text = "foo"
	stale := SearchMatch{FilePath: noLongerMatches, LineNum: 1, LineText: "foo here", ColStart: 1, ColEnd: 4}
	s.Groups = []SearchFileGroup{
		{FilePath: stillMatches, Matches: []SearchMatch{{FilePath: stillMatches, LineNum: 1, LineText: "foo here", ColStart: 1, ColEnd: 4}}},
		{FilePath: noLongerMatches, Matches: []SearchMatch{stale}},
	}

	var got map[string][]SearchMatch
	s.OnReplaceAll = func(allMatches map[string][]SearchMatch, replacement string, opts SearchOptions) { got = allMatches }
	s.ReplaceAllFiles()

	if got == nil {
		t.Fatal("OnReplaceAll was not called")
	}
	if len(got[stillMatches]) == 0 {
		t.Fatalf("still-matching file lost its matches: %+v", got)
	}
	if matches, ok := got[noLongerMatches]; ok {
		t.Fatalf("file the rescan found nothing in should be left out, got %v", matches)
	}
}

// A scan error (here, rg missing from PATH) keeps the panel's own matches
// for every file rather than dropping them.
func TestReplaceAllFilesFallsBackToPanelMatchesOnScanError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no rg on PATH, so the rescan itself errors
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("foo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSearchWidget()
	s.Input.Text = "foo"
	panelMatches := []SearchMatch{{FilePath: path, LineNum: 1, LineText: "foo", ColStart: 1, ColEnd: 4}}
	s.Groups = []SearchFileGroup{{FilePath: path, Matches: panelMatches}}

	var got map[string][]SearchMatch
	s.OnReplaceAll = func(allMatches map[string][]SearchMatch, replacement string, opts SearchOptions) { got = allMatches }
	s.ReplaceAllFiles()

	if len(got[path]) != len(panelMatches) {
		t.Fatalf("expected the panel's matches to be kept, got %v", got[path])
	}
}
