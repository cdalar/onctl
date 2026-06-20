# Getting Started

## initialize
1. `onctl init` is required before using onctl. It creates a `.onctl` directory with a single configuration file. The directory will look like this. 
```
❯ tree
.
└── onctl.yaml

1 directory, 1 file
```
    1. `onctl.yaml` is the single source of truth for every configurable parameter for every provider (global settings, hetzner, aws, gcp, azure, fc), each grouped under its own section, pre-filled with working defaults.
    2. edit the values you want to change; CLI flags (`onctl create --help`) still override whatever is in this file. `gcp.project` and `azure.subscriptionId` ship as placeholders and must be set to use those providers.

## set cloud provider
1. set `ONCTL_CLOUD` environment variables to the name of the cloud provider. Supported values; 
    - azure
    - hetzner
    - aws
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