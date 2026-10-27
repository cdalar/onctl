package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootLoginScript runs the script that lets root in on a cloud VM,
// with sudo stubbed out and the paths pointed at a temp directory. The
// VM's root already has the same key, restricted by cloud-init to print
// "please login as ubuntu" -- the line sshd would match first -- so the
// script must replace it, keep other keys, and allow root key logins in
// both the drop-in and sshd_config.
func TestRootLoginScript(t *testing.T) {
	bin := t.TempDir()
	write(t, filepath.Join(bin, "sudo"), "#!/bin/sh\nshift\nexec \"$@\"\n") // drops -n
	if err := os.Chmod(filepath.Join(bin, "sudo"), 0o755); err != nil {
		t.Fatal(err)
	}
	home, sshd := t.TempDir(), t.TempDir()
	const ours = "ssh-ed25519 AAAAC3ours onctl"
	const other = "ssh-ed25519 AAAAC3other someone-else"
	restricted := `command="echo Please login as ubuntu" ` + ours
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, ".ssh", "authorized_keys"), restricted+"\n"+other+"\n")
	if err := os.MkdirAll(filepath.Join(sshd, "sshd_config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(sshd, "sshd_config"), "Include sshd_config.d/*.conf\n#PermitRootLogin prohibit-password\nPermitRootLogin no\n")

	c := exec.Command("sh", "-c", rootLoginScriptAt(home, sshd, ours))
	c.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	keys, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if got, want := strings.TrimSpace(string(keys)), other+"\n"+ours; got != want {
		t.Errorf("authorized_keys:\n%s\nwant:\n%s", got, want)
	}
	dropIn, err := os.ReadFile(filepath.Join(sshd, "sshd_config.d", "00-onctl-claude.conf"))
	if err != nil || strings.TrimSpace(string(dropIn)) != "PermitRootLogin prohibit-password" {
		t.Errorf("drop-in: %q, %v", dropIn, err)
	}
	conf, _ := os.ReadFile(filepath.Join(sshd, "sshd_config"))
	if strings.Contains(string(conf), "PermitRootLogin no") || !strings.Contains(string(conf), "PermitRootLogin prohibit-password") {
		t.Errorf("sshd_config still forbids root:\n%s", conf)
	}
	// Run again: the key must not be added twice.
	c = exec.Command("sh", "-c", rootLoginScriptAt(home, sshd, ours))
	c.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("second run: %v: %s", err, out)
	}
	keys, _ = os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if n := strings.Count(string(keys), ours); n != 1 {
		t.Errorf("key appears %d times after a second run", n)
	}
}

// TestRemoteURL: git must reach a box through the proxy (the .box host)
// and a VM at its address, with the port only when it isn't ssh's.
func TestRemoteURL(t *testing.T) {
	cases := []struct {
		box  boxSSH
		want string
	}{
		{boxSSH{name: "claude-x", user: "root", host: "claude-x", port: 22}, "root@claude-x.box:/Users/me/x"},
		{boxSSH{name: "claude-x", user: "root", host: "10.0.0.5", port: 22}, "root@10.0.0.5:/Users/me/x"},
		{boxSSH{name: "claude-x", user: "root", host: "10.0.0.5", port: 2222}, "ssh://root@10.0.0.5:2222/Users/me/x"},
	}
	for _, c := range cases {
		if got := c.box.remoteURL("/Users/me/x"); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.box, got, c.want)
		}
	}
}

// TestClaudeSetupScriptInstallsOnlyWhatIsMissing: on the claude-agent
// image everything is there and apt must not run; elsewhere the missing
// tools are installed in one go.
// TestVMClaudeImage: a VM for Claude boots the provider's default image,
// except on ac, which can run the box image with everything baked in --
// the point of the ac default is that nothing gets installed on first use.
func TestVMClaudeImage(t *testing.T) {
	saved := provider
	t.Cleanup(func() { provider = saved })
	provider = &cloud.ProviderAC{}
	assert.Equal(t, acClaudeImage, vmClaudeImage())
	assert.True(t, strings.HasPrefix(acClaudeImage, "ghcr.io/"), "ac needs an OCI image reference, got %q", acClaudeImage)
	provider = &cloud.ProviderHetzner{}
	assert.Equal(t, "", vmClaudeImage(), "a cloud VM takes its provider's configured image")
	assert.False(t, onAC())
}

// TestGoImage: a project with a go.mod gets the Go image wherever the
// default is claude-agent (a box, or ac's ghcr one), so Claude can build
// and test it without installing Go on a small box first. A cloud VM's
// image ("" or one asked for) has no Go counterpart and is kept.
func TestGoImage(t *testing.T) {
	assert.Equal(t, "claude-agent-go", goImage(claudeDefaultImage))
	assert.Equal(t, "ghcr.io/boxctl-io/claude-agent-go:latest", goImage(acClaudeImage))
	assert.Equal(t, "", goImage(""))
	assert.Equal(t, "ubuntu-24.04", goImage("ubuntu-24.04"))
}

// TestProjectHasGoMod: the go.mod is looked for where the project comes
// from -- the local project, or with --repo the repository on GitHub,
// never the directory onctl happens to run in.
func TestProjectHasGoMod(t *testing.T) {
	savedProject, savedRepo, savedAsk := claudeProject, claudeRepo, repoHasGoMod
	t.Cleanup(func() { claudeProject, claudeRepo, repoHasGoMod = savedProject, savedRepo, savedAsk })
	var asked []string
	repoHasGoMod = func(_ context.Context, repo string) bool {
		asked = append(asked, repo)
		return repo == "acme/api"
	}

	goDir, plainDir := t.TempDir(), t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module x\n"), 0o600))
	claudeProject = goDir
	assert.True(t, projectHasGoMod(context.Background()))
	claudeProject = plainDir
	assert.False(t, projectHasGoMod(context.Background()))
	assert.Empty(t, asked, "a local project isn't looked up on GitHub")

	claudeProject = goDir // --repo wins over the local project
	claudeRepo = "acme/web"
	assert.False(t, projectHasGoMod(context.Background()))
	claudeRepo = "acme/api.git"
	assert.True(t, projectHasGoMod(context.Background()))
	assert.Equal(t, []string{"acme/web", "acme/api"}, asked)
}

// TestVMGitHubWithoutHelpers: without the box image's helpers the
// forwarded socket has no reader, so forward turns into off whether it
// was the default or asked for.
func TestVMGitHubWithoutHelpers(t *testing.T) {
	assert.Equal(t, githubOff, vmGitHubWithoutHelpers(false))
	assert.Equal(t, githubOff, vmGitHubWithoutHelpers(true))
	assert.Contains(t, forwardHelpersCheck, "git-credential-boxctl")
}

func TestClaudeSetupScriptInstallsOnlyWhatIsMissing(t *testing.T) {
	run := func(t *testing.T, tools ...string) string {
		bin := t.TempDir()
		for _, name := range append(tools, "claude") {
			write(t, filepath.Join(bin, name), "#!/bin/sh\n")
			if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		write(t, filepath.Join(bin, "apt-get"), "#!/bin/sh\necho \"apt-get $*\" >>\"$LOG\"\n")
		if err := os.Chmod(filepath.Join(bin, "apt-get"), 0o755); err != nil {
			t.Fatal(err)
		}
		log := filepath.Join(t.TempDir(), "log")
		c := exec.Command("sh", "-c", claudeSetupScript())
		// Only the stubs: the machine's own git and curl must not count.
		c.Env = []string{"PATH=" + bin, "HOME=" + t.TempDir(), "LOG=" + log}
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		got, _ := os.ReadFile(log)
		return string(got)
	}
	if got := run(t, "tmux", "git", "curl"); got != "" {
		t.Errorf("apt ran with everything installed: %q", got)
	}
	if got := run(t, "tmux"); !strings.Contains(got, "install -y -qq git curl") {
		t.Errorf("git and curl not installed: %q", got)
	}
}

// A stopped VM that List returns (ac, aws) has no address: onctl claude
// must resume it, not hand it back as is and fail to ssh (issue seen with
// a paused ac box: "claude-onctl has no address to ssh to").
func TestEnsureClaudeVMResumesStoppedVMFromList(t *testing.T) {
	fake := &tuiFakeProvider{vms: []cloud.Vm{
		{Name: "claude-onctl", Status: "stopped", Provider: "ac"},
	}}
	prev := provider
	provider = fake
	t.Cleanup(func() { provider = prev })

	vm, created, err := ensureClaudeVM("claude-onctl", "/dev/null", false)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "10.0.0.9", vm.IP, "the resumed VM's address, not the stopped record's")
	assert.Equal(t, []string{"sshkey", "resume claude-onctl key=key-1"}, fake.calls)
}

func TestEnsureClaudeVMReturnsRunningVMWithoutResuming(t *testing.T) {
	fake := &tuiFakeProvider{vms: []cloud.Vm{
		{Name: "claude-onctl", Status: "running", IP: "192.168.65.4", Provider: "ac"},
	}}
	prev := provider
	provider = fake
	t.Cleanup(func() { provider = prev })

	vm, created, err := ensureClaudeVM("claude-onctl", "/dev/null", false)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "192.168.65.4", vm.IP)
	assert.Empty(t, fake.calls, "a running VM is used as is")
}
