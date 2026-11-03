package cmd

import (
	"errors"

	"github.com/cdalar/onctl/pkg/cloud"
)

// openShellCreateCheck rejects what onctl create can't do on an OpenShell
// sandbox, before one is made: running apply files and cloud-init (no
// root, no file transfer through the relay) and pointing a domain at it
// (it has no address).
func openShellCreateCheck(o cmdCreateOptions) error {
	if len(o.ApplyFiles) > 0 || len(o.UploadFiles) > 0 || len(o.DownloadFiles) > 0 || o.Vm.CloudInitFile != "" {
		return cloud.ErrOpenShellApplyFiles
	}
	if o.Domain != "" {
		return errors.New("--domain points a DNS record at the VM's public IP, and an OpenShell sandbox has none")
	}
	return nil
}

// openShellSSHCheck is openShellCreateCheck for onctl ssh's file options.
func openShellSSHCheck(applyFiles, uploads, downloads []string) error {
	if len(applyFiles) > 0 || len(uploads) > 0 || len(downloads) > 0 {
		return cloud.ErrOpenShellApplyFiles
	}
	return nil
}

// errClaudeOnOpenShell is onctl claude's answer on the openshell provider
// for now (checked before the provider connects, in rootCmd's
// PersistentPreRunE): setting up a box takes root, copying the project takes sftp,
// and Claude needs a network policy that lets it reach its API -- none
// of which an OpenShell sandbox gives onctl yet.
var errClaudeOnOpenShell = errors.New("onctl claude doesn't run on OpenShell sandboxes yet: a sandbox has no root and no file transfer, " +
	"and its default policy blocks Claude's API. Use another provider, or run Claude in a sandbox yourself with the openshell CLI")
