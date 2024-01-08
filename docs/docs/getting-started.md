# Getting Started

## initialize
1. `onctl init` on your project folder. This will create a `.onctl` directory and create files related to each cloud configuration. The directory will look like this. 
```
❯ tree
.
├── azure.yaml
├── hetzner.yaml
├── <cloud provider>.yaml
└── onctl.yaml

1 directory, 3 files
```
    1. `onctl.yaml` file is the main configuration file and holds the all non-cloud specific parameters. 
    2. each provider has it's own configuration yaml file to define things specific things like (*azure resourceGroups*)
    3. change each configuration file depending on your needs. 

## set cloud provider
1. set `ONCTL_CLOUD` environment variables to the name of the cloud provider. Supported values; 
    - azure
    - hetzner
    - aws (coming soon)
1. 
```
export ONCTL_CLOUD=hetzner
```

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