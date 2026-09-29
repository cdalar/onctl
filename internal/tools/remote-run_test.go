package tools

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVariablesToEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		vars     []string
		expected string
	}{
		{
			name:     "Empty input",
			vars:     []string{},
			expected: "",
		},
		{
			name:     "Single variable",
			vars:     []string{"KEY=value"},
			expected: "KEY=\"value\" ",
		},
		{
			name:     "Multiple variables",
			vars:     []string{"KEY1=value1", "KEY2=value2"},
			expected: "KEY1=\"value1\" KEY2=\"value2\" ",
		},
		{
			name:     "Variable with spaces",
			vars:     []string{"KEY=value with spaces"},
			expected: "KEY=\"value with spaces\" ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := variablesToEnvVars(tt.vars)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestApplyCommandSurvivesLostSession runs applyCommand's script locally
// and drops the reader after the first line, the way a Ctrl+C or a lost
// connection drops the SSH session. The apply file must still run to the
// end: a half-applied install (the result of a plain `| tee`, which dies
// of SIGPIPE) is worse than no output at all.
func TestApplyCommandSurvivesLostSession(t *testing.T) {
	if err := exec.Command("tail", "--pid=1", "-n", "0", os.DevNull).Run(); err != nil {
		t.Skip("needs GNU tail --pid:", err)
	}
	dir := t.TempDir()
	bin := t.TempDir()
	// Stand-in for sudo, so the test needs no root.
	writeExecutable(t, filepath.Join(bin, "sudo"), "#!/bin/bash\n[ \"$1\" = -E ] && shift\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(dir, "apply.sh"), "#!/bin/bash\necho first\nsleep 1\necho \"last $FOO\"\nexit 3\n")

	cmd := exec.Command("bash", "-c", applyCommand(dir, "apply.sh", variablesToEnvVars([]string{"FOO=bar"})))
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "first\n" {
		t.Fatalf("first streamed line = %q, %v; want \"first\\n\"", line, err)
	}
	_ = stdout.Close()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	exitCodeFile := filepath.Join(dir, "exit-code-apply.sh")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b, err := os.ReadFile(exitCodeFile); err == nil && strings.TrimSpace(string(b)) == "3" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("apply file did not finish after the session went away")
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, err := os.ReadFile(filepath.Join(dir, "output-apply.sh.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "first\nlast bar\n" {
		t.Errorf("log = %q, want the apply file's whole output", out)
	}
	// What a later session would pass to tail --pid to follow the log.
	pid, err := os.ReadFile(filepath.Join(dir, "pid-apply.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.Atoi(strings.TrimSpace(string(pid))); err != nil {
		t.Errorf("pid file = %q, want a PID", pid)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}
