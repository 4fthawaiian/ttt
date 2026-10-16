import { describe, it, expect, afterEach } from "vitest";
import { mkdirSync, writeFileSync, readFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import * as tui from "./tui.js";
import { createTempDir, createTempFile, cleanupDir } from "./helpers.js";

let dir;

afterEach(() => {
  tui.kill();
  if (dir) cleanupDir(dir);
});

function setup() {
  dir = createTempDir();
  const ws = join(dir, "ws");
  const home = join(dir, "home");
  const configDir = join(dir, "config");
  mkdirSync(join(home, "omarchy"), { recursive: true });
  mkdirSync(join(configDir, "themes"), { recursive: true });
  writeFileSync(join(home, "omarchy", "colors.toml"), 'background = "#1a1b26"\n');
  const pluginDir = join(ws, "plugins", "fsprobe");
  mkdirSync(pluginDir, { recursive: true });
  const file = createTempFile(ws, "main.txt", "workspace file\n");
  writeFileSync(
    join(pluginDir, "plugin.ttt.json"),
    JSON.stringify({
      name: "fsprobe",
      version: "0.1.0",
      entry: "init.lua",
      permissions: {
        commands: true,
        "fs.read": ["~/omarchy"],
        "fs.write": ["{config}/themes"],
      },
    }),
  );
  writeFileSync(
    join(pluginDir, "init.lua"),
    `local ttt = require("ttt")
local fs = require("ttt.fs")
local function result(value, err)
  if value then return "ok" end
  return err and err:match("access denied") and "denied" or tostring(err)
end
ttt.register({
  commands = {
    { id = "fsprobe.run", title = "FS Probe", handler = function()
      ttt.log("info", "palette=" .. tostring(fs.read("~/omarchy/colors.toml")))
      ttt.log("info", "workspace=" .. result(fs.read(${JSON.stringify(file)})))
      ttt.log("info", "theme=" .. result(fs.write("{config}/themes/omarchy.json", '{"name":"omarchy"}')))
      ttt.log("info", "home-write=" .. result(fs.write("~/omarchy/colors.toml", "x")))
      ttt.log("info", "probe-done")
    end },
  },
})
`,
  );
  return { ws, file, home, configDir };
}

describe("plugin fs paths", () => {
  it("grants exactly the manifest's paths and lists them for approval", () => {
    const { ws, file, home, configDir } = setup();

    tui.start(ws, file);
    tui.setEnv({ HOME: home, TTT_CONFIG_DIR: configDir });
    tui.waitFor("{config}/themes");
    const approval = tui.snapshot();
    tui.press("a");
    tui.waitFor("workspace file");
    tui.exec("FS Probe");
    tui.panel("output");
    tui.waitFor("probe-done");
    const output = tui.snapshot();
    const { snapshots } = tui.run();

    expect(snapshots[approval]).toMatch(/Read files\s+~\/omarchy/);
    expect(snapshots[approval]).toMatch(/Write files\s+\{config\}\/themes/);
    expect(snapshots[output]).toContain('palette=background = "#1a1b26"');
    expect(snapshots[output]).toContain("workspace=denied");
    expect(snapshots[output]).toContain("theme=ok");
    expect(snapshots[output]).toContain("home-write=denied");

    const theme = join(configDir, "themes", "omarchy.json");
    expect(existsSync(theme)).toBe(true);
    expect(readFileSync(theme, "utf8")).toBe('{"name":"omarchy"}');
    expect(readFileSync(join(home, "omarchy", "colors.toml"), "utf8")).toBe('background = "#1a1b26"\n');
  });
});
