package cmd

import (
	"errors"

	"github.com/cdalar/onctl/pkg/cloud"
)

// openShellSSHCheck refuses onctl ssh's file options on an OpenShell
// sandbox (box.Client.CheckSetup does the same for create).
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
