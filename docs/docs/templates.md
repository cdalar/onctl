---
sidebar_position: 6
---
# Templates

A template is a bash script onctl runs on a VM over ssh: on a new VM with
`onctl create -a`, or on one that already exists with `onctl ssh NAME -a`.
Cloud-init is the other way in, at boot.

```bash
onctl create -n web -a docker/docker.sh           # a new VM, Docker installed
onctl ssh web -a scripts/harden.sh                # the same on an existing VM
onctl create -n web -i cloud-init/ssh-443.config  # cloud-init at boot
```

## Where a script can come from

`-a` takes a path or a URL, resolved in this order:

1. **A local file**: `onctl create -a scripts/init.sh` runs `scripts/init.sh`
   from your current directory.
2. **A file embedded in onctl** (under `internal/files/` in the repository).
3. **The [onctl-templates](https://github.com/cdalar/onctl-templates)
   repository**, by relative path: `onctl create -a wireguard/vpn.sh` fetches
   `https://templates.onctl.com/wireguard/vpn.sh`. `onctl templates list`
   shows what's there.
4. **Any URL**: `onctl create -a https://example.com/setup.sh`.

`-a` can be given more than once; the scripts run in order.

## Files and variables for the script

| | |
|---|---|
| `-e KEY=VALUE`, `--vars` | environment variables for the script (repeatable) |
| `--dot-env .env` | the same, from a dot-env file |
| `-u`, `--upload FILE` | files copied to the VM before the scripts run (repeatable) |
| `-d`, `--download FILE` | files copied back here after they've run (repeatable) |

They work the same on `onctl create` and `onctl ssh`.

## cloud-init

`-i`/`--cloud-init` hands a [cloud-init](https://cloudinit.readthedocs.io/en/latest/)
file to the VM at boot, before any `-a` script runs. For example, to move ssh
to port 443 and then install WireGuard:

```bash
onctl create -a wireguard/vpn.sh --cloud-init cloud-init-ssh-443.config -P 443
```

`--cloud-init-timeout` (default `3m`) is how long onctl waits for cloud-init
to finish before running the scripts.

## Everything in one file

`-f`/`--file` takes all of `create`'s options from a YAML file, so a VM's
setup can be kept and repeated:

```yaml
vm:
  name: web
  type: cpx21
  image: ubuntu-24.04
applyFiles:
  - docker/docker.sh
  - scripts/deploy.sh
uploadFiles:
  - docker-compose.yml
variables:
  - APP_ENV=production
dotEnvFile: .env
```

```bash
onctl create -f web.yaml
onctl env create -t web.yaml      # the same
onctl env destroy -t web.yaml     # and its teardown
```
