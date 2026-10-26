---
sidebar_position: 2
---
# Getting Started

## Install

```bash
curl -fsSL https://onctl.sh/get.sh | bash
```

That installs the latest release to `~/.local/bin` and sets up tab completion;
the [Onctl](./) page has the details, the edge build and Windows.

## Initialize

`onctl init` is required before using onctl. It always ensures a global
`~/.onctl` directory exists, and, if run interactively, asks whether to also
create a project-local `.onctl` in the current directory (its settings override
the global ones). Either way you end up with a directory that looks like this:

```
❯ tree
.onctl
└── onctl.yaml

1 directory, 1 file
```

`onctl.yaml` is the single source of truth for every configurable parameter
for every provider: global settings (`vm`, `ssh`, `actions`), then a section
each for `hetzner`, `aws`, `gcp`, `azure`, `ovh`, `fc`, `ch` and `ac`,
pre-filled with working defaults. Edit the values you want to change; CLI flags
(`onctl create --help`) still override whatever is in this file.
`gcp.project`, `azure.subscriptionId` and `ovh.serviceName` ship as
placeholders and must be set to use those providers.

## Pick a provider

Set `ONCTL_CLOUD`, or pass `-p`/`--provider` on any command:

```bash
export ONCTL_CLOUD=hetzner
# or, per command
onctl -p hetzner ls
```

| Provider | What it is | What it needs |
|---|---|---|
| `hetzner` | Hetzner Cloud | `HCLOUD_TOKEN` |
| `aws` | Amazon EC2 | the usual AWS credentials (`AWS_PROFILE`, or `AWS_ACCESS_KEY_ID` and friends) |
| `azure` | Azure VMs | an `az login`; `azure.subscriptionId` and a resource group (`--subscription-id`, `--resource-group`, or the `az` CLI's defaults) |
| `gcp` | Google Compute Engine | `gcloud` credentials (or `GOOGLE_CREDENTIALS`); `gcp.project`, or `--project`, falling back to `gcloud config get-value project` |
| `ovh` | OVH Public Cloud | a one-time consumer key from [api.ovh.com/createToken](https://api.ovh.com/createToken/), then `OVH_APPLICATION_KEY`, `OVH_APPLICATION_SECRET`, `OVH_CONSUMER_KEY`, and `ovh.serviceName` (or `--service-name`) |
| `boxes` | hosted microVMs from [boxctl.io](https://boxctl.io), no cloud account and no host to run | `onctl login` once with a token from the dashboard (`ONCTL_BOXES_TOKEN` overrides it, for CI) |
| `fc` | local [Firecracker](https://firecracker-microvm.github.io) microVMs | Linux with KVM, the `firecracker` binary, a kernel and rootfs image (`fc.kernelImage`, `fc.rootfsImage`) |
| `ch` | local [Cloud Hypervisor](https://github.com/cloud-hypervisor/cloud-hypervisor) microVMs | Linux with KVM, the `cloud-hypervisor` binary, a kernel and rootfs image (`ch.kernelImage`, `ch.rootfsImage`); no pause/resume yet |
| `ac` | local sandboxes on a Mac with [Apple container](https://github.com/apple/container): each a lightweight Linux VM on Virtualization.framework | macOS 26+, the `container` CLI and `container system start`; any OCI image via `--image` (default `ubuntu:24.04`), sshd set up on first boot; `pause`/`resume` are `container stop`/`start` |
| `static` | servers onctl didn't create, registered with `onctl import` | ssh access to them |

:::note
With `ONCTL_CLOUD` unset and no `-p`, onctl picks the first provider whose
credentials it finds in the environment, in this order: aws, azure, gcp,
hetzner, ovh. The local and hosted providers (`fc`, `ch`, `ac`, `boxes`,
`static`) always have to be named. `onctl claude` is the exception: with
nothing set it uses `boxes`.
:::

## Create a VM

```
❯ onctl create -n onctl-test
Using: hetzner
Creating SSHKey: onctl-xxx...
SSH Key already exists (onctl-xxx)
Starting server...
Server IP: x.x.x.x
Vm started.
```

`up` and `start` are aliases. The useful flags:

| | |
|---|---|
| `-n`, `--name` | the VM's name (a random one otherwise) |
| `--type` | its size; `onctl sizes` lists what the provider offers, with prices |
| `--image` | its OS image; `onctl images` lists them (on `ac`, any OCI image) |
| `--location` | region (Hetzner default `fsn1`) |
| `-a`, `--apply-file` | bash script(s) to run on it once it's up, see [Templates](./templates) |
| `-i`, `--cloud-init` | a cloud-init file to boot it with |
| `-u`, `--upload` / `-d`, `--download` | files to copy up before the scripts, or down after |
| `-e`, `--vars` / `--dot-env` | environment variables for the scripts |
| `-k`, `--publicKey` | the public key to install (default `~/.ssh/id_rsa.pub`) |
| `-P`, `--ssh-port` / `--username` | how to reach it afterwards |
| `--domain` | a DNS name for it, through Cloudflare (`CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ZONE_ID`) |
| `-f`, `--file` | all of the above from a YAML file |

Firecracker and Cloud Hypervisor VMs take their resources on the command line
too: `--vcpu`, `--memory` (MiB), `--rootfs-size`, `--kernel-image`,
`--rootfs-image`, and for `fc` a persistent `--cache-image` as a second
drive.

## Connect

```
❯ onctl ssh onctl-test
Using: hetzner
root@onctl-test:~#
```

`onctl ssh NAME -- COMMAND` runs one command instead of opening a shell.
`onctl ssh NAME -a script.sh` runs a script on an existing VM the way
`create -a` does on a new one, with the same `-u`/`-d`/`-e` flags. Boxes have
no public address; `onctl ssh` reaches them through the service, and plain
`ssh` can too with `onctl ssh-proxy` as its `ProxyCommand`.

To reach a port on the VM from here, like `ssh -L` but for any provider:

```bash
onctl port-forward my-vm 3000              # http://localhost:3000 is the VM's 3000
onctl pf my-vm 15432:5432 0:8080           # Postgres on 15432, the VM's 8080 on any free port
onctl pf my-vm 5432:db.internal:5432       # through the VM, to a host only it can reach
```

## List, pause, resume, destroy

```bash
onctl ls                 # running VMs, then paused ones; -o json|yaml|puppet|ansible
onctl pause my-vm        # snapshot the disk, keep the IP, delete the server: no compute cost
onctl resume my-vm       # recreate it from the snapshot, IP re-attached where the provider allows
onctl destroy my-vm      # gone, after asking (-f skips the question)
onctl restart my-vm      # fc only: cold-boot it from its own disk, optionally --kernel-image a new kernel
```

`pause --hot` snapshots without shutting down first (faster, crash-consistent).
On `ac`, pause and resume stop and start the container; `ch` has no
pause/resume yet.

## The rest

- **[Terminal UI](./tui)**: `onctl tui` is all of the above as a k9s-style
  view: `s` shell, `a` Claude Code, `c` create, `p`/`r` pause and resume,
  `ctrl-d` destroy, `:images`, `:sizes`, `:repos`, `:provider`.
- **[Claude Code on a box](./claude)**: `onctl claude` runs Claude Code on a
  box with your project on it.
- **Import a server onctl didn't create**, a Hetzner auction box say, so
  `ls` and `ssh` work on it too: `onctl import NAME --ip 1.2.3.4 --user root
  --key ~/.ssh/key`, then `onctl -p static ssh NAME`. It's written to
  `~/.ssh/onctl_config`, so plain `ssh NAME` works as well. `destroy` on an
  imported host only forgets it locally.
- **Actions**: `onctl action NAME` downloads and runs a binary from the
  `cdalar/onctl-action-NAME` repository (`--github-owner` for another), with
  `-j params.json` on its stdin.
- **Environments**: `onctl env create -t env.yaml` is `onctl create -f
  env.yaml`; `onctl env destroy -t env.yaml` tears it down.
