package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/viper"
)

// onctl claude on a VM of any provider but boxes: `onctl -p hetzner
// claude` (aws, fc, ac, ...) creates the project's VM through the
// provider, as onctl create does, and reaches it over plain ssh at its
// address with the key onctl.yaml names (ssh.privateKey) -- the follow-up
// docs/plans/boxes-provider.md ("onctl claude") describes. Boxes stay the
// default: they need nothing set up, and their image has the GitHub
// helpers --github forward relies on. A VM gets tmux, git and curl from
// apt on first use (claudeSetupScript) and Claude Code from its installer.
//
// Everything on the VM runs as root, like on a box, so the project can go
// to the same absolute path and the box-side scripts stay the same. Where
// the provider's user isn't root (aws's ubuntu, gcp, azure), the first
// connection uses that user to let root in with the same key.

// claudeSSHPoll is how often a booting VM is tried.
const claudeSSHPoll = 3 * time.Second

// onBoxes reports whether the selected provider is boxes.
func onBoxes() bool {
	_, ok := provider.(*cloud.ProviderBoxes)
	return ok
}

// onAC reports whether the selected provider is ac (Apple containers).
func onAC() bool {
	_, ok := provider.(*cloud.ProviderAC)
	return ok
}

// vmClaudeImage is the image a VM for Claude is made from when --image
// isn't given: on ac, which runs OCI images the way a box does, the
// claude-agent image; elsewhere "" for the provider's configured default.
func vmClaudeImage() string {
	if onAC() {
		return acClaudeImage
	}
	return ""
}

// forwardHelpersCheck tells whether a VM has the box image's GitHub
// forwarding helpers (git-credential-boxctl and the gh wrapper beside it).
const forwardHelpersCheck = "test -x /usr/local/bin/git-credential-boxctl"

// resolveVMGitHub settles --github forward on a VM once it's reachable:
// it stays where the VM has the box image's helpers (the claude-agent
// image, on ac) and becomes off where it doesn't -- the helpers are what
// the forwarded socket is for. explicit is whether --github was given.
// Boxes always have them.
func resolveVMGitHub(ctx context.Context, box *boxSSH, explicit bool) {
	if onBoxes() || claudeGitHub != githubForward {
		return
	}
	if box.run(ctx, forwardHelpersCheck, nil) == nil {
		return
	}
	claudeGitHub = vmGitHubWithoutHelpers(explicit)
}

// vmGitHubWithoutHelpers is what --github forward becomes on a VM without
// the helpers, said more pointedly when forward was asked for.
func vmGitHubWithoutHelpers(explicit bool) string {
	if explicit {
		fmt.Fprintf(os.Stderr, "--github forward needs the box image's helpers, which this %s VM doesn't have; Claude gets no GitHub credentials (--github store keeps a token there).\n", cloudProvider)
	} else {
		fmt.Fprintf(os.Stderr, "Claude on %s gets no GitHub credentials (--github store keeps a token there).\n", cloudProvider)
	}
	return githubOff
}

// connectClaudeVM finds or creates the VM named name on the selected
// provider and returns a root ssh connection to it, reporting whether it
// created it.
func connectClaudeVM(ctx context.Context, name string, create bool) (box *boxSSH, created bool, err error) {
	publicKeyFile, privateKeyFile := getSSHKeyFilePaths("")
	if _, err := os.Stat(privateKeyFile); err != nil {
		return nil, false, fmt.Errorf("ssh key for %s VMs (ssh.privateKey in onctl.yaml): %w", cloudProvider, err)
	}
	vm, created, err := ensureClaudeVM(name, publicKeyFile, create)
	if err != nil {
		return nil, false, err
	}
	if vm.IP == "" {
		return nil, false, fmt.Errorf("%s has no address to ssh to", name)
	}
	user := viper.GetString(cloudProvider + ".vm.username")
	if user == "" {
		user = "root"
	}
	port := vm.SSHPort
	if port == 0 {
		port = 22
	}
	ssh, err := newVMSSH(name, user, vm.IP, port, privateKeyFile)
	if err != nil {
		return nil, false, err
	}
	if err := waitSSH(ctx, ssh, claudeReadyTimeout); err != nil {
		return nil, false, err
	}
	if user == "root" {
		return ssh, created, nil
	}
	// The provider's user gets root in with the same key, then it's root
	// from here on.
	pub, err := os.ReadFile(publicKeyFile)
	if err != nil {
		ssh.close()
		return nil, false, err
	}
	err = ssh.run(ctx, rootLoginScript(strings.TrimSpace(string(pub))), nil)
	ssh.close()
	if err != nil {
		return nil, false, fmt.Errorf("letting root in on %s as %s: %w", name, user, err)
	}
	root, err := newVMSSH(name, "root", vm.IP, port, privateKeyFile)
	if err != nil {
		return nil, false, err
	}
	if err := root.probe(ctx); err != nil {
		root.close()
		return nil, false, fmt.Errorf("can't ssh into %s as root after enabling it (%s allows key logins for root?); set %s.vm.username to root in onctl.yaml, or allow PermitRootLogin prohibit-password there", name, cloudProvider, cloudProvider)
	}
	return root, created, nil
}

// ensureClaudeVM returns the provider's VM named name: the existing one,
// a paused one resumed, or -- when create is set -- a new one from --size
// and --image where given, else the provider's configured defaults.
func ensureClaudeVM(name, publicKeyFile string, create bool) (vm cloud.Vm, created bool, err error) {
	list, err := provider.List()
	if err != nil {
		return cloud.Vm{}, false, err
	}
	var stopped bool
	for _, vm := range list.List {
		// A "dead" fc/ch record is a VMM process that died out of band;
		// Deploy makes a new one in its place (as onctl create does).
		if vm.Name != name || vm.Status == "dead" {
			continue
		}
		// Some providers (ac, aws) list stopped VMs rather than filing
		// them under ListPaused; such a VM has no address until resumed.
		if isPausedStatus(vm.Status) {
			stopped = true
			break
		}
		return vm, false, nil
	}
	keyID, err := provider.CreateSSHKey(publicKeyFile)
	if err != nil {
		return cloud.Vm{}, false, err
	}
	if stopped {
		return resumeClaudeVM(name, keyID)
	}
	paused, err := provider.ListPaused()
	if err != nil {
		return cloud.Vm{}, false, err
	}
	for _, vm := range paused.List {
		if vm.Name == name {
			return resumeClaudeVM(name, keyID)
		}
	}
	if !create {
		return cloud.Vm{}, false, fmt.Errorf("no VM %s on %s -- start one with onctl -p %s claude", name, cloudProvider, cloudProvider)
	}
	want := cloud.Vm{Name: name, SSHKeyID: keyID, SSHPort: 22, Type: claudeSize, Image: claudeImage}
	// Some providers read these from the config rather than the Vm.
	if claudeSize != "" {
		viper.Set(cloudProvider+".vm.type", claudeSize)
	}
	if claudeImage != "" {
		viper.Set(cloudProvider+".vm.image", claudeImage)
	}
	fmt.Fprintln(os.Stderr, creatingMessage(name))
	vm, err = provider.Deploy(want)
	if err != nil {
		return cloud.Vm{}, false, fmt.Errorf("creating %s: %w", name, err)
	}
	return vm, true, nil
}

// resumeClaudeVM resumes the paused VM named name through the provider.
func resumeClaudeVM(name, keyID string) (cloud.Vm, bool, error) {
	fmt.Fprintf(os.Stderr, "Resuming %s...\n", name)
	vm, err := provider.Resume(cloud.Vm{Name: name, SSHKeyID: keyID})
	if err != nil {
		return cloud.Vm{}, false, fmt.Errorf("resuming %s: %w", name, err)
	}
	return vm, false, nil
}

// waitSSH tries ssh to the VM until it answers or timeout passes: a VM
// just created or resumed may still be booting.
func waitSSH(ctx context.Context, ssh *boxSSH, timeout time.Duration) error {
	if ssh.probe(ctx) == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "Waiting for ssh on %s (%s)...\n", ssh.name, ssh.host)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(claudeSSHPoll):
		}
		if ssh.probe(ctx) == nil {
			return nil
		}
	}
	return fmt.Errorf("%s didn't answer ssh at %s within %s", ssh.name, ssh.host, timeout)
}

// probe checks the connection, quietly: a booting VM refuses or resets it.
func (b *boxSSH) probe(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ssh", append(append([]string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}, b.args...), "true")...)
	return cmd.Run()
}

// rootLoginScript lets root in with pubkey, run with sudo as the
// provider's user. Cloud images that make a user put the launch key in
// root's authorized_keys too, prefixed with command="echo Please login as
// ..." -- and sshd takes the first line that matches a key, so a line of
// ours after it would never be reached: lines for this key are replaced,
// not added to. PermitRootLogin is set to prohibit-password (keys only)
// in a drop-in that sorts first, and in sshd_config itself where there is
// no drop-in directory.
func rootLoginScript(pubkey string) string {
	return rootLoginScriptAt("/root", "/etc/ssh", pubkey)
}

func rootLoginScriptAt(home, sshd, pubkey string) string {
	body := pubkey
	if fields := strings.Fields(pubkey); len(fields) >= 2 {
		body = fields[1]
	}
	ak := shellQuote(home + "/.ssh/authorized_keys")
	return `set -e
sudo -n mkdir -p ` + shellQuote(home+"/.ssh") + ` && sudo -n chmod 700 ` + shellQuote(home+"/.ssh") + `
{ sudo -n cat ` + ak + ` 2>/dev/null | grep -vF ` + shellQuote(body) + ` || true; echo ` + shellQuote(pubkey) + `; } | sudo -n tee ` + ak + `.onctl >/dev/null
sudo -n mv ` + ak + `.onctl ` + ak + ` && sudo -n chmod 600 ` + ak + `
if [ -d ` + shellQuote(sshd+"/sshd_config.d") + ` ]; then echo 'PermitRootLogin prohibit-password' | sudo -n tee ` + shellQuote(sshd+"/sshd_config.d/00-onctl-claude.conf") + ` >/dev/null; fi
sudo -n sh -c 'sed "s/^#*[[:space:]]*PermitRootLogin.*/PermitRootLogin prohibit-password/" "$1" >"$1.onctl" && mv "$1.onctl" "$1"' sh ` + shellQuote(sshd+"/sshd_config") + `
sudo -n systemctl reload ssh 2>/dev/null || sudo -n systemctl reload sshd 2>/dev/null || sudo -n service ssh reload 2>/dev/null || true`
}

// claudeVM is a VM of the provider as onctl claude ls shows it.
type claudeVM struct {
	name, state string
}

// listClaudeVMs lists the provider's VMs with their state, "running" for
// one Claude can be reached on.
func listClaudeVMs(ctx context.Context) ([]claudeVM, error) {
	if onBoxes() {
		vms, err := newClient().List(ctx)
		if err != nil {
			return nil, err
		}
		var out []claudeVM
		for _, vm := range vms {
			out = append(out, claudeVM{name: vm.Name, state: vm.State})
		}
		return out, nil
	}
	list, err := provider.List()
	if err != nil {
		return nil, err
	}
	paused, err := provider.ListPaused()
	if err != nil {
		return nil, err
	}
	var out []claudeVM
	for _, vm := range append(list.List, paused.List...) {
		out = append(out, claudeVM{name: vm.Name, state: strings.ToLower(vm.Status)})
	}
	return out, nil
}
