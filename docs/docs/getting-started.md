# Getting Started

## initialize
1. `onctl init` is required before using onctl. It always ensures a global `~/.onctl` directory exists, and -- if run interactively -- asks whether to also create a project-local `.onctl` in the current directory (its settings override the global ones). Either way you end up with a directory that looks like this:
```
❯ tree
.onctl
└── onctl.yaml

1 directory, 1 file
```
    1. `onctl.yaml` is the single source of truth for every configurable parameter for every provider (global settings, hetzner, aws, gcp, azure, ovh, fc, ch), each grouped under its own section, pre-filled with working defaults.
    2. edit the values you want to change; CLI flags (`onctl create --help`) still override whatever is in this file. `gcp.project`, `azure.subscriptionId` and `ovh.serviceName` ship as placeholders and must be set to use those providers.

## set cloud provider
1. set `ONCTL_CLOUD` environment variable (or pass `-p`/`--provider` on any command) to the name of the cloud provider. Supported values; 
    - aws
    - azure
    - gcp
    - hetzner
    - ovh (requires a one-time consumer-key setup — see [api.ovh.com/createToken](https://api.ovh.com/createToken/) — then set `OVH_APPLICATION_KEY`, `OVH_APPLICATION_SECRET`, `OVH_CONSUMER_KEY`)
    - fc (local Firecracker microVM, no cloud account needed)
    - ch (local Cloud Hypervisor microVM, no cloud account needed)
1. 
```
export ONCTL_CLOUD=hetzner
```

!!! note 

    If you don't set ONCTL_CLOUD environment variable, onctl tool will try to find credentials on your shell and use the first one it finds. 

## spin up a virtual machine
1. We're ready. Let's create a Virtual Machine (Instance) 
```
❯ onctl up -n onctl-test
Using: hetzner
Creating SSHKey: onctl-xxx...
SSH Key already exists (onctl-xxx)
Starting server...
Server IP: x.x.x.x
Vm started.
```
## ssh access
1. Just use ssh command to ssh into the virtual machine. 
```
❯ onctl ssh onctl-test
Using: hetzner
.
.
.
root@onctl-test:~# 
```