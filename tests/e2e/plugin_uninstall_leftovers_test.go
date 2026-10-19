package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eugenioenko/ttt/internal/plugin"
)

// The probe plugin from #972: a command sets a status item, and on_uninstall
// opens a tab.
const leftoverProbeInit = `
	local ttt = require("ttt")
	ttt.register({ commands = { { id = "probe.mark", title = "Probe: Mark", handler = function()
		ttt.set_status_item("left", "probe", "PROBESTATUS")
	end } } })
	ttt.on_uninstall(function()
		ttt.open_tab({ title = "PROBETAB", render = function(panel) panel:label("left behind") end })
	end)
`

func loadLeftoverProbe(h *testHarness) *plugin.Manager {
	h.t.Helper()
	cfg := filepath.Join(h.dir, "config")
	dir := filepath.Join(cfg, "plugins", "probe")
	if err := os.MkdirAll(dir, 0755); err != nil {
		h.t.Fatal(err)
	}
	files := map[string]string{
		"plugin.ttt.json": `{"name": "probe", "version": "1.0.0", "entry": "init.lua", "permissions": {"commands": true, "panel.editor": true}}`,
		"init.lua":        leftoverProbeInit,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			h.t.Fatal(err)
		}
	}
	m := plugin.NewManager(filepath.Join(cfg, "plugins"), filepath.Join(cfg, "plugins.ttt.json"))
	h.app.PluginManager = m
	h.t.Cleanup(m.Shutdown)
	pending := m.LoadAll()
	if len(pending) != 1 {
		h.t.Fatalf("pending approvals = %d, want 1", len(pending))
	}
	if err := m.ApproveAndLoad(pending[0]); err != nil {
		h.t.Fatal(err)
	}
	h.app.WirePlugin(pending[0])
	h.redraw()
	return m
}

func (h *testHarness) confirmUninstall(name string) {
	h.t.Helper()
	h.app.PluginUninstallByName(name)
	h.redraw()
	for y := 0; y < 24; y++ {
		row := h.screenRow(y)
		if strings.Contains(row, "Cancel") {
			if x := displayColumnOf(row, "Uninstall"); x >= 0 {
				h.click(x+1, y)
				h.redraw()
				return
			}
		}
	}
	h.t.Fatal("no Uninstall button on screen")
}

func TestPluginUninstallRemovesItsStatusItems(t *testing.T) {
	h := newTestHarness(t, 80, 24)
	defer h.stop()
	m := loadLeftoverProbe(h)

	h.exec("probe.mark")
	h.assertContains("PROBESTATUS")

	h.confirmUninstall("probe")

	if m.FindPlugin("probe") != nil {
		t.Fatal("plugin still installed")
	}
	h.assertNotContains("PROBESTATUS")
}

func TestPluginUninstallClosesTabsItsUninstallCallbackOpens(t *testing.T) {
	h := newTestHarness(t, 80, 24)
	defer h.stop()
	m := loadLeftoverProbe(h)
	tabs := h.app.EditorGroup.TabCount()

	h.confirmUninstall("probe")

	if m.FindPlugin("probe") != nil {
		t.Fatal("plugin still installed")
	}
	if got := h.app.EditorGroup.TabCount(); got != tabs {
		t.Errorf("tab count = %d after uninstall, want %d", got, tabs)
	}
	h.assertNotContains("PROBETAB")
}

func TestPluginReloadDropsTheOldInstancesStatusItems(t *testing.T) {
	h := newTestHarness(t, 80, 24)
	defer h.stop()
	loadLeftoverProbe(h)

	h.exec("probe.mark")
	h.assertContains("PROBESTATUS")

	h.exec("plugin.reload")
	h.assertNotContains("PROBESTATUS")

	h.exec("probe.mark")
	h.assertContains("PROBESTATUS")
}
