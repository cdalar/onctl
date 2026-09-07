---
sidebar_position: 1
---
# Onctl

`onctl` is a tool to manage virtual machines in multi-cloud. 

Check 🌍 https://docs.onctl.io for detailed documentation

[![build](https://github.com/cdalar/onctl/actions/workflows/build.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/build.yml)
[![Lint](https://github.com/cdalar/onctl/actions/workflows/lint.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/lint.yml)
[![CodeQL](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql)
[![codecov](https://codecov.io/gh/cdalar/onctl/graph/badge.svg?token=7VU7H1II09)](https://codecov.io/gh/cdalar/onctl)
![Github All Releases](https://img.shields.io/github/downloads/cdalar/onctl/total.svg)
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/cdalar/onctl?sort=semver)
<!-- [![Known Vulnerabilities](https://snyk.io/test/github/cdalar/onctl/main/badge.svg)](https://snyk.io/test/github/cdalar/onctl/main) -->

## What onctl brings 

- 🌍 Simple intuitive CLI to run VMs in seconds.  
- ⛅️ Supports multi cloud providers (aws, azure, hetzner, more coming soon...)
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
curl -sLS https://docs.onctl.io/get.sh | bash
sudo install onctl /usr/local/bin/
```

#### Edge build (latest `main`, Linux/Windows)

To install or update to the `edge` build, an unsigned binary rebuilt from the tip of `main` on every push (no macOS build):

```bash
curl -sLS https://docs.onctl.io/get_edge.sh | bash
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

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  create      Create a VM
  destroy     Destroy VM(s)
  help        Help about any command
  init        init onctl environment
  ls          List VMs
  ssh         Spawn an SSH connection to a VM
  version     Print the version number of onctl

Flags:
  -h, --help   help for onctl

Use "onctl [command] --help" for more information about a command.
```
