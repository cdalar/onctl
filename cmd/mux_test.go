package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHerdr puts a herdr on PATH that logs its calls to the returned
// file and answers like herdr 0.9.5 does: `workspace list` fails until
// `server` has run, then lists the workspaces `workspace create` made,
// labeled and working; `workspace create` returns its root pane. A setsid
// goes with it, for a machine without one (macOS).
func fakeHerdr(t *testing.T) (log string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
d=` + dir + `
echo "$@" >>"$d/log"
case "$1 $2" in
"workspace list")
  [ -f "$d/up" ] || { echo '{"error":{"code":"server_not_running"}}' >&2; exit 1; }
  ws=""; n=0
  for l in $(cat "$d/labels" 2>/dev/null); do
    n=$((n+1)); ws="$ws${ws:+,}{\"active_tab_id\":\"w$n:t1\",\"agent_status\":\"working\",\"focused\":true,\"label\":\"$l\",\"number\":$n,\"pane_count\":1,\"tab_count\":1,\"workspace_id\":\"w$n\"}"
  done
  printf '{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[%s]}}\n' "$ws" ;;
"server ") touch "$d/up" ;;
"workspace close") grep -v "^$(sed -n "${3#w}p" "$d/labels")$" "$d/labels" >"$d/labels.new"; mv "$d/labels.new" "$d/labels" ;;
"workspace create")
  while [ $# -gt 0 ]; do [ "$1" = --label ] && l=$2; shift; done
  echo "$l" >>"$d/labels"; n=$(wc -l <"$d/labels" | tr -d ' ')
  printf '{"id":"cli:workspace:create","result":{"root_pane":{"agent_status":"unknown","cwd":"/x","focused":true,"pane_id":"w%s:p1","tab_id":"w%s:t1","workspace_id":"w%s"},"type":"workspace_created"}}\n' "$n" "$n" "$n" ;;
esac
`
	for name, body := range map[string]string{"herdr": script, "setsid": "#!/bin/sh\nexec \"$@\"\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(dir, "log")
}

// boxPaths points the box-side paths at a temporary directory for one
// test.
func boxPaths(t *testing.T) (marker, runDir string) {
	t.Helper()
	dir := t.TempDir()
	marker, runDir = filepath.Join(dir, "boxctl", "mux"), filepath.Join(dir, "run")
	oldMarker, oldRunDir := muxMarker, muxRunDir
	muxMarker, muxRunDir = marker, runDir
	t.Cleanup(func() { muxMarker, muxRunDir = oldMarker, oldRunDir })
	return marker, runDir
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestHerdrStart runs the herdr start script for real against a fake
// herdr: the first start brings the server up, makes the session's
// workspace in its directory and types the command into it; the second
// finds the workspace and only focuses it; both attach. The pane only
// runs the command: a box here outlives its runs, so the server is left
// to the box's other sessions when Claude exits.
func TestHerdrStart(t *testing.T) {
	log := fakeHerdr(t)
	marker, runDir := boxPaths(t)
	s := newTaskSession("/w/my app", "")
	line := `exec claude 'fix "it"'`
	start := func(detached bool) string {
		out, err := exec.Command("sh", "-c", (herdrMux{}).start(s, line, detached)).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return readFile(t, log)
	}

	calls := start(false)
	if got := readFile(t, marker); got != "herdr" {
		t.Errorf("marker: %q", got)
	}
	if got := readFile(t, filepath.Join(runDir, "claude.cmd")); got != line+"\n" {
		t.Errorf("command file: %q", got)
	}
	for _, want := range []string{
		"server\n",
		"integration install claude\n",
		"workspace create --cwd /w/my app --label claude --focus\n",
		"pane run w1:p1 exec sh " + shellQuote(runDir+"/claude.cmd") + "\n",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("first start: no %q in:\n%s", want, calls)
		}
	}
	if strings.Contains(calls, "server stop") {
		t.Errorf("the server must outlive the session:\n%s", calls)
	}
	if !strings.HasSuffix(calls, "\n\n") { // `exec herdr`: the client, no arguments
		t.Errorf("first start doesn't end attached:\n%s", calls)
	}

	calls = strings.TrimPrefix(start(false), calls)
	if strings.Contains(calls, "workspace create") || !strings.Contains(calls, "workspace focus w1\n") || !strings.HasSuffix(calls, "\n\n") {
		t.Errorf("second start should only focus and attach:\n%s", calls)
	}
}

// TestHerdrStartEphemeral: an ephemeral run deletes its box once Claude
// exits, so the attached client must return then -- the pane stops the
// server after Claude. Detached (a kept box) it doesn't.
func TestHerdrStartEphemeral(t *testing.T) {
	log := fakeHerdr(t)
	_, runDir := boxPaths(t)
	s := newTaskSession("/w/app", "")
	s.ephemeral = true
	out, err := exec.Command("sh", "-c", (herdrMux{}).start(s, "exec claude", false)).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	stopFile := runDir + "/claude.stop"
	pane := "sh " + shellQuote(runDir+"/claude.cmd") + "; [ ! -e " + shellQuote(stopFile) + " ] || herdr server stop"
	if calls := readFile(t, log); !strings.Contains(calls, "pane run w1:p1 "+pane+"\n") {
		t.Errorf("ephemeral pane:\n%s", calls)
	}
	// Claude exiting (here: claude not found) runs the rest of the pane.
	if out, err := exec.Command("sh", "-c", pane).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.HasSuffix(readFile(t, log), "server stop\n") {
		t.Error("ephemeral pane must stop the server after Claude, so the attached client returns and the box can go")
	}
	// A run that detaches keeps its box and removes the stop file
	// (herdrStopFile): the box's other sessions must outlive this one.
	if err := os.Remove(stopFile); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, log)
	_ = exec.Command("sh", "-c", pane).Run()
	if strings.Contains(strings.TrimPrefix(readFile(t, log), before), "server stop") {
		t.Error("a kept box's pane stopped herdr's server, killing its other sessions")
	}
	log = fakeHerdr(t)
	_, runDir = boxPaths(t)
	if out, err := exec.Command("sh", "-c", (herdrMux{}).start(s, "exec claude", true)).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if calls := readFile(t, log); strings.Contains(calls, "server stop") || !strings.Contains(calls, "pane run w1:p1 exec sh "+shellQuote(runDir+"/claude.cmd")+"\n") {
		t.Errorf("a detached start must leave the server running:\n%s", calls)
	}
	if _, err := os.Stat(runDir + "/claude.stop"); err == nil {
		t.Error("a detached start made a stop file")
	}
}

// TestClaudeSessions: --mux against the box's other multiplexer is
// refused while any Claude session runs there, not just the one being
// started -- starting moves the box's record, and the others would be
// looked for in the wrong place (shown stopped, started a second time).
// Sessions that aren't Claude's don't count.
func TestClaudeSessions(t *testing.T) {
	got := claudeSessions(tmuxMux{}, "work 1700000000\nclaude-auth 1700000001\nclaudex 1700000002\nclaude 1700000003\n")
	if strings.Join(got, ",") != "claude,claude-auth" {
		t.Errorf("tmux: %v", got)
	}
	list := `{"result":{"workspaces":[{"agent_status":"idle","label":"claude-auth","workspace_id":"w1"},{"agent_status":"-","label":"notes","workspace_id":"w2"}]}}`
	if got := claudeSessions(herdrMux{}, list); strings.Join(got, ",") != "claude-auth" {
		t.Errorf("herdr: %v", got)
	}
	if got := claudeSessions(herdrMux{}, ""); len(got) != 0 {
		t.Errorf("no server: %v", got)
	}
}

// TestHerdrStartDetached: a detached start makes a task's workspace
// under the task's label and doesn't attach.
func TestHerdrStartDetached(t *testing.T) {
	log := fakeHerdr(t)
	_, runDir := boxPaths(t)
	s := newTaskSession("/w/app", "auth")
	out, err := exec.Command("sh", "-c", (herdrMux{}).start(s, "exec claude", true)).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	calls := readFile(t, log)
	if !strings.Contains(calls, "--label claude-auth --focus\n") || !strings.Contains(calls, "pane run w1:p1 exec sh "+shellQuote(runDir+"/claude-auth.cmd")+"\n") {
		t.Errorf("task session:\n%s", calls)
	}
	if strings.HasSuffix(calls, "\n\n") {
		t.Errorf("detached start attaches:\n%s", calls)
	}
}

func TestHerdrRunningAndList(t *testing.T) {
	s := newTaskSession("/w/app", "")
	if got := (herdrMux{}).running(s); !strings.Contains(got, `'"label":"claude"'`) {
		t.Error(got)
	}
	list := `{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[` +
		`{"active_tab_id":"w1:t1","agent_status":"working","focused":true,"label":"claude","number":1,"pane_count":1,"tab_count":1,"workspace_id":"w1"},` +
		`{"active_tab_id":"w2:t1","agent_status":"blocked","focused":false,"label":"claude-auth","number":2,"pane_count":1,"tab_count":1,"workspace_id":"w2"}]}}`
	got := (herdrMux{}).parseList(list)
	if got["claude"] != "working" || got["claude-auth"] != "blocked" || len(got) != 2 {
		t.Errorf("parseList: %v", got)
	}
	if got := (herdrMux{}).parseList(""); len(got) != 0 {
		t.Errorf("no server: %v", got)
	}
}

func TestTmuxList(t *testing.T) {
	got := (tmuxMux{}).parseList("claude 1700000000\nclaude-auth 1700000001\nbroken\n")
	if len(got) != 2 || !strings.HasSuffix(got["claude"], " ago") {
		t.Errorf("parseList: %v", got)
	}
	if got := (tmuxMux{}).running(newTaskSession("/w/app", "")); got != "tmux has-session -t =claude 2>/dev/null" {
		t.Error(got)
	}
}

func TestMuxNamed(t *testing.T) {
	for name, want := range map[string]string{"tmux": "tmux", "herdr": "herdr"} {
		m, err := muxNamed(name)
		if err != nil || m.name() != want {
			t.Errorf("%s: %v %v", name, m, err)
		}
	}
	if _, err := muxNamed("screen"); err == nil {
		t.Error("screen accepted")
	}
}

// TestDefaultMux: ONCTL_MUX sets --mux's default; without it, tmux.
func TestDefaultMux(t *testing.T) {
	t.Setenv(muxEnv, "")
	if got := defaultMux(); got != "tmux" {
		t.Errorf("default: %q", got)
	}
	t.Setenv(muxEnv, "herdr")
	if got := defaultMux(); got != "herdr" {
		t.Errorf("with %s: %q", muxEnv, got)
	}
}

// TestProjectCheckReportsTheMux: the project check also says which
// multiplexer the box's sessions run in, when one has started.
func TestProjectCheckReportsTheMux(t *testing.T) {
	marker, _ := boxPaths(t)
	proj := t.TempDir()
	projectMarker := filepath.Join(t.TempDir(), "project")
	status, boxMux := parseProjectCheck(sh(t, projectCheckScriptAt(projectMarker, proj)))
	if status != "ok" || boxMux != "" {
		t.Errorf("before any session: %q %q", status, boxMux)
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("herdr"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, boxMux = parseProjectCheck(sh(t, projectCheckScriptAt(projectMarker, proj)))
	if status != "ok" || boxMux != "herdr" {
		t.Errorf("with a herdr session: %q %q", status, boxMux)
	}
}

// TestHerdrStartAfterServerRestart: a server started afresh restores
// the old layout, so the start closes what it restored and makes the
// session's workspace anew, rather than taking the restored shell for
// the session.
func TestHerdrStartAfterServerRestart(t *testing.T) {
	log := fakeHerdr(t)
	boxPaths(t)
	s := newTaskSession("/w/app", "")
	start := func() string {
		out, err := exec.Command("sh", "-c", (herdrMux{}).start(s, "exec claude", true)).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return readFile(t, log)
	}
	first := start()
	if err := os.Remove(filepath.Join(filepath.Dir(log), "up")); err != nil { // the server died; its labels are its saved layout
		t.Fatal(err)
	}
	calls := strings.TrimPrefix(start(), first)
	if !strings.Contains(calls, "server\n") || !strings.Contains(calls, "workspace close w1\n") || !strings.Contains(calls, "workspace create --cwd /w/app --label claude --focus\n") {
		t.Errorf("after a restart:\n%s", calls)
	}
	if i, j := strings.Index(calls, "workspace close w1"), strings.Index(calls, "workspace create"); i > j {
		t.Errorf("the restored workspace must go before the session's is made:\n%s", calls)
	}
}

// TestSetupScriptInstallsTheMux: with --mux herdr the setup installs
// herdr when the box lacks it, and leaves it be when it's there; tmux's
// setup never touches herdr.
func TestSetupScriptInstallsTheMux(t *testing.T) {
	old := mux
	t.Cleanup(func() { mux = old })
	mux = herdrMux{}
	if s := claudeSetupScript(); !strings.Contains(s, "herdr.dev/install.sh") || !strings.Contains(s, "command -v herdr") {
		t.Errorf("herdr setup missing:\n%s", s)
	}
	mux = tmuxMux{}
	if s := claudeSetupScript(); strings.Contains(s, "herdr") {
		t.Errorf("tmux setup mentions herdr:\n%s", s)
	}
}
