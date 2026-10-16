package app

import (
	"testing"

	"github.com/eugenioenko/ttt/internal/ui"
)

func TestDiffSearchSourcesCarryPRFileIconPath(t *testing.T) {
	const diffText = "--- a/Dockerfile\n+++ b/Dockerfile\n@@ -1 +1 @@\n-FROM base\n+FROM alpine\n"
	group := ui.NewEditorGroupWidget(nil, 4, false, "extended")
	app := &App{
		EditorGroup: group,
		Changes: &ChangesPanel{PRGroups: []prGroup{{
			PRDiffs: map[string]string{"Dockerfile": diffText},
		}}},
	}

	sources := app.DiffSearchSources()
	if len(sources) != 1 {
		t.Fatalf("diff sources = %d, want 1", len(sources))
	}
	if sources[0].TabName != "Dockerfile (diff)" || sources[0].IconPath != "Dockerfile" {
		t.Fatalf("PR diff source = %+v, want display tab ID and real file path", sources[0])
	}
}
