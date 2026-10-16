package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eugenioenko/ttt/internal/git"
	"github.com/gdamore/tcell/v3"
)

func TestNewBranchCommandCreatesBranchInDisplayedRepositories(t *testing.T) {
	h := newTestHarness(t, 100, 30)
	defer h.stop()

	workspaceRoot := filepath.Join(h.dir, "repos")
	repoDirs := []string{
		filepath.Join(workspaceRoot, "first"),
		filepath.Join(workspaceRoot, "second"),
	}
	for _, dir := range repoDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		gitIn(t, dir, "init", "-q")
		gitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	}
	plainDir := filepath.Join(workspaceRoot, "not-a-repo")
	if err := os.MkdirAll(plainDir, 0755); err != nil {
		t.Fatal(err)
	}
	h.app.Changes.SetDirs([]string{repoDirs[0], workspaceRoot, plainDir})

	const branch = "feature/from-changes-panel"
	h.exec("git.newBranch")
	h.assertContains("Branch name")
	for _, r := range branch {
		h.pressRune(r)
	}
	h.pressKey(tcell.KeyEnter, tcell.ModNone)

	result := h.awaitRepoOp()
	if result.Err != nil {
		t.Fatalf("creating branch: %v", result.Err)
	}
	for _, dir := range repoDirs {
		if got := git.BranchName(dir); got != branch {
			t.Errorf("current branch in %q = %q, want %q", dir, got, branch)
		}
	}
	h.assertContains("Created branch: " + branch)
}
