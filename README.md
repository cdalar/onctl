# Onctl

`onctl` is a tool to manage virtual machines in multi-cloud. 

Check 🌍 https://docs.onctl.io for detailed documentation

[![build](https://github.com/cdalar/onctl/actions/workflows/build.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/build.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/cdalar/onctl/badge)](https://scorecard.dev/viewer/?uri=github.com/cdalar/onctl)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/10052/badge)](https://www.bestpractices.dev/projects/10052)
[![Go Report Card](https://goreportcard.com/badge/github.com/cdalar/onctl)](https://goreportcard.com/report/github.com/cdalar/onctl)
[![CodeQL](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql)
[![codecov](https://codecov.io/gh/cdalar/onctl/graph/badge.svg?token=7VU7H1II09)](https://codecov.io/gh/cdalar/onctl)
[![Github All Releases](https://img.shields.io/github/downloads/cdalar/onctl/total.svg)]()
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/cdalar/onctl?sort=semver)
<!-- [![Known Vulnerabilities](https://snyk.io/test/github/cdalar/onctl/main/badge.svg)](https://snyk.io/test/github/cdalar/onctl/main) -->

## What onctl brings 

- 🌍 Simple intuitive CLI to run VMs in seconds.  
- ⛅️ Supports multi cloud providers (aws, azure, gcp, hetzner, more coming soon...)
- 🔥 Run local microVMs with the `fc` (Firecracker) provider (Linux + KVM, no cloud account needed). No bare-metal Linux box? Create a nested-virtualization-enabled GCP VM (`gcp.vm.nestedVirtualization: true`) with `onctl create -n fc-host -a firecracker/firecracker-host-setup.sh`, then SSH in and run `ONCTL_CLOUD=fc onctl create -n my-microvm`.
- 🚀 Sets your public key and Gives you SSH access with `onctl ssh <vm-name>`
- ✨ Cloud-init support. Set your own cloud-init file `onctl up -n qwe --cloud-init <cloud.init.file>`
- 🤖 Use ready to use templates to configure your vm. Check [onctl-templates](https://github.com/cdalar/onctl-templates) `onctl up -n qwe -a k3s/k3s-server.sh`
- 🗂️ Use your custom local or http accessible scripts to configure your vm. `onctl ssh qwe -a <my_local_script.sh>`
- 📥 Import a server onctl didn't create (e.g. a Hetzner auction/dedicated box) so you can `ssh`/`ls` it too: `onctl import <name> --ip <ip>`
  
## Quick Start

initialize project. this will create a `.onctl` directory. check configuration file and set as needed.
```
❯ onctl init
onctl environment initialized
```

export `ONCTL_CLOUD` to set Cloud Provider. 
```
❯ export ONCTL_CLOUD=hetzner
```

Be sure that credentials for that specific cloud provider is already set. 
If you already use cloud provider CLI. They're already . ex. `az`, `aws`, `hcloud`
```
❯ echo $HCLOUD_TOKEN
```

Create VM.
```
❯ onctl up -n onctl-test
Using: hetzner
Creating SSHKey: onctl-42da32a9...
SSH Key already exists (onctl-42da32a9)
Starting server...
Server IP: 168.119.58.112
Vm started.
```

Ssh into VM.
```
❯ onctl ssh onctl-test
Using: hetzner
Welcome to Ubuntu 22.04.3 LTS (GNU/Linux 5.15.0-89-generic x86_64)
.
.
.
root@onctl-test:~# 
```

## Import an existing server

Already have a server onctl didn't create — a Hetzner auction/dedicated box (these live on Hetzner's Robot API, a different product from Hetzner Cloud, so the `hetzner` provider can never see them), or any other reachable host? Register it so `ssh`/`ls` work against it too:
```
❯ onctl import myauctionbox --ip 1.2.3.4 --user root --key ~/.ssh/auction_key
✔ Imported "myauctionbox" (1.2.3.4)
Use it with: onctl --provider static ssh myauctionbox

❯ onctl --provider static ls
❯ onctl --provider static ssh myauctionbox
```
Imported hosts only support `ssh`/`ls`/`destroy` — `destroy` just forgets the host locally, it never touches the real machine, since onctl didn't create it and has no lifecycle API for it.

## Installation

### MacOS

```zsh
brew install cdalar/tap/onctl
```

#### Dev build (latest `main`)

To install or update `onctl-dev`, a build of the latest `main` branch (installed alongside the regular `onctl`):

```zsh
./scripts/install-dev.sh
```

Or directly via Homebrew:

```zsh
brew install --HEAD --fetch-HEAD cdalar/tap/onctl-dev
```

### Linux

```bash
curl -sLS https://docs.onctl.io/get.sh | bash
sudo install onctl /usr/local/bin/
```

### Windows 

- download windows binary from [releases page](https://github.com/cdalar/onctl/releases)
- unzip and copy onctl.exe to a location in PATH

# Enjoy ✅

```
❯ onctl
onctl is a tool to manage cross platform resources in cloud

Usage:
  onctl [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  create      Create a VM
  destroy     Destroy VM(s)
  help        Help about any command
  import      Import an existing server so it can be managed with ssh/ls
  init        init onctl environment
  ls          List VMs
  ssh         Spawn an SSH connection to a VM
  version     Print the version number of onctl

Flags:
  -h, --help   help for onctl

Use "onctl [command] --help" for more information about a command.
```

## Contributing

For development guidelines — especially when using AI coding agents (opencode, Claude, etc.) — see [AGENTS.md](AGENTS.md).

**Important:** When running multiple AI agents in parallel on this repo, you must use separate git worktrees (or clones) for isolation. See the "Coordinating multiple agents..." section in AGENTS.md.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=cdalar/onctl&type=Date)](https://star-history.com/#cdalar/onctl&Date)
