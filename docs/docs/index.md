---
sidebar_position: 1
---
# Onctl

`onctl` is a tool to manage virtual machines in multi-cloud. 

Check 🌍 https://onctl.sh for detailed documentation

[![build](https://github.com/cdalar/onctl/actions/workflows/build.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/build.yml)
[![Lint](https://github.com/cdalar/onctl/actions/workflows/lint.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/lint.yml)
[![CodeQL](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql)
[![codecov](https://codecov.io/gh/cdalar/onctl/graph/badge.svg?token=7VU7H1II09)](https://codecov.io/gh/cdalar/onctl)
![Github All Releases](https://img.shields.io/github/downloads/cdalar/onctl/total.svg)
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/cdalar/onctl?sort=semver)
<!-- [![Known Vulnerabilities](https://snyk.io/test/github/cdalar/onctl/main/badge.svg)](https://snyk.io/test/github/cdalar/onctl/main) -->

## What onctl brings 

- 🌍 Simple intuitive CLI to run VMs in seconds.  
- ⛅️ Supports multiple cloud providers (aws, azure, gcp, hetzner, ovh) plus local microVMs via Firecracker (`fc`) and Cloud Hypervisor (`ch`) -- no cloud account needed for those two
- 🚀 Sets your public key and Gives you SSH access with `onctl ssh <vm-name>`
- ✨ Cloud-init support. Set your own cloud-init file `onctl up -n qwe --cloud-init <cloud.init.file>`
- 🤖 Use ready to use templates to configure your vm. Check [onctl-templates](https://github.com/cdalar/onctl-templates) `onctl up -n qwe -a k3s/k3s-server.sh`
- 🗂️ Use your custom local or http accessible scripts to configure your vm. `onctl ssh qwe -a <my_local_script.sh>`
  
## Quick Start

initialize project. this will create a `.onctl` directory. check configuration file and set as needed.
```bash
❯ onctl init
onctl environment initialized
```

### Mac OS

```zsh
brew install cdalar/tap/onctl
```

### Linux

```bash
curl -sLS https://onctl.sh/get.sh | bash
sudo install onctl /usr/local/bin/
```

#### Edge build (latest `main`, Linux only)

To install or update to the `edge` build, an unsigned binary rebuilt from the tip of `main` on every push (no macOS or Windows build):

```bash
curl -sLS https://onctl.sh/get_edge.sh | bash
sudo install onctl /usr/local/bin/
```

### Windows 

- download windows binary from [releases page](https://github.com/cdalar/onctl/releases)
- unzip and copy onctl.exe to a location in PATH

# Enjoy ✅

```bash
❯ onctl
onctl is a tool to manage cross platform resources in cloud

Usage:
  onctl [command]

Examples:
  # List all VMs
  onctl ls

  # Create a VM with docker installed
  onctl create -n test -a docker/docker.sh

  # SSH into a VM
  onctl ssh test

  # Destroy a VM
  onctl destroy test

Available Commands:
  action      Execute a custom action from GitHub
  completion  Generate the autocompletion script for the specified shell
  create      Create a VM
  deploy      Deploy a Docker image to a remote VM
  destroy     Destroy VM(s)
  env         Manage environments
  help        Help about any command
  images      List available OS images for the current cloud provider
  import      Import an existing server so it can be managed with ssh/ls
  init        init onctl environment
  ls          List VMs
  pause       Snapshot and delete a VM to stop compute cost (keeps its IP)
  resume      Recreate a paused VM from its snapshot
  ssh         Spawn an SSH connection to a VM
  templates   Manage onctl templates
  version     Print the version number of onctl
  vm          Manage virtual machines

Flags:
  -c, --config string                             Path to onctl.yaml configuration file (overrides the .onctl directory lookup)
  -h, --help                                       help for onctl
      --project gcloud config get-value project   GCP: project ID (falls back to gcloud config get-value project when the onctl.yaml placeholder is present)
  -p, --provider string                           cloud provider: aws, hetzner, azure, gcp, ovh, fc, ch, static (overrides ONCTL_CLOUD)
      --resource-group string                     Azure: resource group (required for the azure provider; falls back to the az CLI's configured default group, if any)
      --service-name string                       OVH: Public Cloud project service name (required for the ovh provider)
      --subscription-id az account show           Azure: subscription ID (required for the azure provider; falls back to az account show)

Use "onctl [command] --help" for more information about a command.
```
