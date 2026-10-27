package plugin

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// mockSpawnedProcess is a test SpawnedProcess: configurable streams and exit
// code, recorded writes, and signal channels for synchronization with the
// plugin's pump/writer goroutines.
type mockSpawnedProcess struct {
	stdin  *bytes.Buffer
	stdout io.Reader
	stderr io.Reader
	code   int

	mu          sync.Mutex
	killed      bool
	stdinClosed bool

	wrote  chan struct{}
	closed chan struct{}
	waited chan struct{}
}

func (m *mockSpawnedProcess) Write(data []byte) (int, error) {
	n, err := m.stdin.Write(data)
	if m.wrote != nil {
		select {
		case m.wrote <- struct{}{}:
		default:
		}
	}
	return n, err
}

func (m *mockSpawnedProcess) CloseStdin() error {
	m.mu.Lock()
	m.stdinClosed = true
	m.mu.Unlock()
	if m.closed != nil {
		close(m.closed)
	}
	return nil
}

func (m *mockSpawnedProcess) Kill() error {
	m.mu.Lock()
	m.killed = true
	m.mu.Unlock()
	return nil
}

func (m *mockSpawnedProcess) killedState() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.killed
}

func (m *mockSpawnedProcess) PID() int          { return 4242 }
func (m *mockSpawnedProcess) Stdout() io.Reader { return m.stdout }
func (m *mockSpawnedProcess) Stderr() io.Reader { return m.stderr }

func (m *mockSpawnedProcess) Wait() (int, error) {
	if m.waited != nil {
		close(m.waited)
	}
	return m.code, nil
}

// postChan routes async results to a channel so tests can run callbacks on
// the test goroutine, matching the production UI-thread delivery.
func postChan() (func(*PluginAsyncResult), chan *PluginAsyncResult) {
	ch := make(chan *PluginAsyncResult, 64)
	return func(r *PluginAsyncResult) { ch <- r }, ch
}

// runPosted runs up to n posted callbacks, waiting for each.
func runPosted(t *testing.T, ch chan *PluginAsyncResult, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case r := <-ch:
			r.Callback()
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for async callback %d of %d", i+1, n)
		}
	}
}

func waitFor(t *testing.T, ch chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestSpawnDeniedWithoutExecGrant(t *testing.T) {
	p, cleanup := newTestPluginBase(PermissionSet{})
	defer cleanup()

	err := p.State.DoString(`
		local sys = require("ttt.system")
		_G.spawn_fn = sys.spawn
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.State.GetGlobal("spawn_fn") != lua.LNil {
		t.Error("expected sys.spawn to be nil without system.exec grant")
	}
}

func TestSpawnDeniedBinary(t *testing.T) {
	mock := &mockSystemAPI{}
	p, cleanup := setupTestPluginWithSystem(
		PermissionSet{SystemExec: []string{"docker"}},
		mock,
	)
	defer cleanup()

	err := p.State.DoString(`
		local sys = require("ttt.system")
		local proc, err = sys.spawn("rm", {})
		_G.proc = proc
		_G.err = err
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.State.GetGlobal("proc") != lua.LNil {
		t.Error("expected nil proc for a binary outside the allowlist")
	}
	if p.State.GetGlobal("err") == lua.LNil {
		t.Error("expected an error string")
	}
	if mock.spawnBinary != "" {
		t.Error("host Spawn should not have been called")
	}
}

func TestSpawnPassesOptions(t *testing.T) {
	mock := &mockSystemAPI{}
	p, cleanup := setupTestPluginWithSystem(
		PermissionSet{SystemExec: []string{"devin"}},
		mock,
	)
	defer cleanup()

	err := p.State.DoString(`
		local sys = require("ttt.system")
		local proc, err = sys.spawn("devin", {
			args = {"acp"},
			cwd = "/tmp/ws",
			env = {FOO = "bar"},
		})
		assert(proc ~= nil, tostring(err))
		_G.pid = proc:pid()
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if mock.spawnBinary != "devin" {
		t.Errorf("expected binary 'devin', got %q", mock.spawnBinary)
	}
	if len(mock.spawnArgs) != 1 || mock.spawnArgs[0] != "acp" {
		t.Errorf("unexpected args: %v", mock.spawnArgs)
	}
	if mock.spawnOpts.Cwd != "/tmp/ws" {
		t.Errorf("unexpected cwd: %q", mock.spawnOpts.Cwd)
	}
	if len(mock.spawnOpts.Env) != 1 || mock.spawnOpts.Env[0] != "FOO=bar" {
		t.Errorf("unexpected env: %v", mock.spawnOpts.Env)
	}
	if p.State.GetGlobal("pid") != lua.LNumber(4242) {
		t.Errorf("unexpected pid: %v", p.State.GetGlobal("pid"))
	}
}

func TestSpawnSpawnError(t *testing.T) {
	mock := &mockSystemAPI{spawnErr: io.EOF}
	p, cleanup := setupTestPluginWithSystem(
		PermissionSet{SystemExec: []string{"devin"}},
		mock,
	)
	defer cleanup()

	err := p.State.DoString(`
		local sys = require("ttt.system")
		local proc, err = sys.spawn("devin", {})
		_G.proc = proc
		_G.err = err
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.State.GetGlobal("proc") != lua.LNil {
		t.Error("expected nil proc on spawn error")
	}
	if p.State.GetGlobal("err") == lua.LNil {
		t.Error("expected an error string")
	}
}

func TestSpawnStreamsToCallbacks(t *testing.T) {
	mp := &mockSpawnedProcess{
		stdin:  &bytes.Buffer{},
		stdout: strings.NewReader("hello stdout"),
		stderr: strings.NewReader("oops stderr"),
		code:   7,
	}
	mock := &mockSystemAPI{spawnProc: mp}
	p, cleanup := setupTestPluginWithSystem(
		PermissionSet{SystemExec: []string{"cat"}},
		mock,
	)
	defer cleanup()
	post, ch := postChan()
	p.PostAsync = post

	err := p.State.DoString(`
		local sys = require("ttt.system")
		_G.out, _G.errs, _G.code = {}, {}, nil
		local proc = sys.spawn("cat", {
			on_stdout = function(c) table.insert(_G.out, c) end,
			on_stderr = function(c) table.insert(_G.errs, c) end,
			on_exit = function(code) _G.code = code end,
		})
		assert(proc ~= nil)
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// Each stream delivers one chunk, then both EOFs trigger on_exit.
	runPosted(t, ch, 3)

	out := p.State.GetGlobal("out").(*lua.LTable)
	if out.Len() != 1 || out.RawGetInt(1).String() != "hello stdout" {
		t.Errorf("unexpected stdout chunks: %v", out)
	}
	errs := p.State.GetGlobal("errs").(*lua.LTable)
	if errs.Len() != 1 || errs.RawGetInt(1).String() != "oops stderr" {
		t.Errorf("unexpected stderr chunks: %v", errs)
	}
	if p.State.GetGlobal("code") != lua.LNumber(7) {
		t.Errorf("unexpected exit code: %v", p.State.GetGlobal("code"))
	}
}

func TestProcWriteAndCloseStdin(t *testing.T) {
	mp := &mockSpawnedProcess{
		stdin:  &bytes.Buffer{},
		stdout: strings.NewReader(""),
		stderr: strings.NewReader(""),
		wrote:  make(chan struct{}, 8),
		closed: make(chan struct{}),
	}
	mock := &mockSystemAPI{spawnProc: mp}
	p, cleanup := setupTestPluginWithSystem(
		PermissionSet{SystemExec: []string{"cat"}},
		mock,
	)
	defer cleanup()

	err := p.State.DoString(`
		local sys = require("ttt.system")
		_G.proc = sys.spawn("cat", {})
		assert(_G.proc ~= nil)
		assert(_G.proc:write("line one\n") == true)
		assert(_G.proc:write("line two\n") == true)
		assert(_G.proc:close_stdin() == true)
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	waitFor(t, mp.wrote, "stdin write")
	waitFor(t, mp.closed, "stdin close")
	if got := mp.stdin.String(); got != "line one\nline two\n" {
		t.Errorf("unexpected stdin: %q", got)
	}
}

func TestProcWriteAfterCloseFails(t *testing.T) {
	mp := &mockSpawnedProcess{
		stdin:  &bytes.Buffer{},
		stdout: strings.NewReader(""),
		stderr: strings.NewReader(""),
	}
	mock := &mockSystemAPI{spawnProc: mp}
	p, cleanup := setupTestPluginWithSystem(
		PermissionSet{SystemExec: []string{"cat"}},
		mock,
	)
	defer cleanup()

	err := p.State.DoString(`
		local sys = require("ttt.system")
		local proc = sys.spawn("cat", {})
		proc:close_stdin()
		-- Wait a tick for the close to be marked, then write must fail.
		_G.ok, _G.err = proc:write("too late")
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.State.GetGlobal("ok") != lua.LNil {
		t.Error("expected write after close_stdin to fail")
	}
	if p.State.GetGlobal("err") == lua.LNil {
		t.Error("expected an error string")
	}
}

func TestDestroyKillsSpawnedProcesses(t *testing.T) {
	mp := &mockSpawnedProcess{
		stdin:  &bytes.Buffer{},
		stdout: strings.NewReader(""),
		stderr: strings.NewReader(""),
	}
	mock := &mockSystemAPI{spawnProc: mp}
	p, _ := setupTestPluginWithSystem(
		PermissionSet{SystemExec: []string{"cat"}},
		mock,
	)
	// No cleanup: Destroy closes the Lua state itself.

	err := p.State.DoString(`
		local sys = require("ttt.system")
		_G.proc = sys.spawn("cat", {})
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	p.Destroy()
	if !mp.killedState() {
		t.Error("expected spawned process to be killed on plugin destroy")
	}
}
