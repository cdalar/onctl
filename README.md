# Onctl

`onctl` is a tool to manage virtual machines in multi-cloud. 

Check 🌍 https://onctl.com for detailed documentation

[![build](https://github.com/cdalar/onctl/actions/workflows/build.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/cdalar/onctl)](https://goreportcard.com/report/github.com/cdalar/onctl)
[![CodeQL](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/github-code-scanning/codeql)
[![codecov](https://codecov.io/gh/cdalar/onctl/graph/badge.svg?token=7VU7H1II09)](https://codecov.io/gh/cdalar/onctl)
[![Github All Releases](https://img.shields.io/github/downloads/cdalar/onctl/total.svg)]()
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/cdalar/onctl?sort=semver)
<!-- [![Known Vulnerabilities](https://snyk.io/test/github/cdalar/onctl/main/badge.svg)](https://snyk.io/test/github/cdalar/onctl/main) -->

## What onctl brings 

- 🌍 Simple intuitive CLI to run VMs in seconds.  
- ⛅️ Supports multi cloud providers (azure, hetzner, more coming soon...)
- 🚀 Gives you SSH access with you ssh public key precofigured. So no need to manage username/password
- ✨ Cloud-init support. Set your own cloud-init file

## Installation

### MacOS

```zsh
brew install cdalar/tap/onctl
```

### Linux

```bash
curl -sLS https://www.onctl.com/get.sh | bash
sudo install onctl /usr/local/bin/
```

### Windows 

- download windows binary from [releases page](https://github.com/cdalar/onctl/releases)
- unzip and copy onctl.exe to a location in PATH

# Getting Started

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
  init        init onctl environment
  ls          List VMs
  ssh         Spawn an SSH connection to a VM
  version     Print the version number of onctl

Flags:
  -h, --help   help for onctl

Use "onctl [command] --help" for more information about a command.
```
