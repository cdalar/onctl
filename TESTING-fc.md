# Testing the `fc` provider (this branch)

How to test the renamed Firecracker provider (`firecracker` → `fc`) and its new
zero-YAML CLI flags, on a GCP `fc-host` VM. The host-setup script already
installs KVM + the `firecracker` binary + kernel/rootfs images under
`~/.onctl/firecracker/images/`; this guide tests the **new `onctl` binary**
(renamed provider `fc` + new flags) talking to that host.

## 1. Build this branch for the host (linux/amd64)

From your workstation, on branch `firecracker-flags-no-yaml`:

```bash
GOOS=linux GOARCH=amd64 go build -o onctl-fc .
```

## 2. Provision / reuse the fc-host (if you don't have one)

```bash
# nested-virt GCP VM with KVM + firecracker + images under ~/.onctl/firecracker/images
ONCTL_CLOUD=gcp onctl create -n fc-host -a firecracker/firecracker-host-setup.sh
```

If `fc-host` already exists, skip this.

## 3. Copy the new binary onto the host

```bash
HOST_IP=$(ONCTL_CLOUD=gcp onctl ls | awk '/fc-host/{print $4}')   # confirm column for IP
scp onctl-fc <user>@$HOST_IP:/tmp/onctl
onctl ssh fc-host          # or: ssh <user>@$HOST_IP
sudo install /tmp/onctl /usr/local/bin/onctl   # replace any old release
onctl version
```

## 4. Run the `fc` provider on the host

`fc` needs `CAP_NET_ADMIN` (tap/bridge) → run as **root**. Note that
`~/.onctl/firecracker` is per-user, so under `sudo` it resolves to
`/root/.onctl/...`. Two options:

**A — point at the images the setup script downloaded (recommended):**
```bash
KIMG=$HOME/.onctl/firecracker/images/vmlinux
RIMG=$HOME/.onctl/firecracker/images/rootfs.ext4

sudo onctl create -n mv1 --provider fc \
  --kernel-image "$KIMG" --rootfs-image "$RIMG"
```

**B — zero-config (tests the new defaults):** put images at
`/root/.onctl/firecracker/images/{vmlinux,rootfs.ext4}`, then just:
```bash
sudo ONCTL_CLOUD=fc onctl create -n mv1
```
This exercises the Phase-1 defaults (`fc.kernelImage`/`fc.rootfsImage` now
default to `~/.onctl/firecracker/images/...` with no YAML).

## 5. Verify the microVM and the rename

```bash
sudo ONCTL_CLOUD=fc onctl ls            # Provider column should read "fc"
sudo ONCTL_CLOUD=fc onctl ls | grep mv1 # grab its IP
ssh root@<microvm-ip>                   # your pubkey was injected into the rootfs

# new flags work:
sudo onctl create -n mv2 --provider fc --vcpu 2 --memory 1024 \
  --kernel-image "$KIMG" --rootfs-image "$RIMG"

# pause / resume:
sudo ONCTL_CLOUD=fc onctl pause mv2 && sudo ONCTL_CLOUD=fc onctl resume mv2

# cleanup:
sudo ONCTL_CLOUD=fc onctl destroy mv1 -f && sudo ONCTL_CLOUD=fc onctl destroy mv2 -f
```

## 6. Confirm the old name is gone (the actual rename check)

```bash
sudo ONCTL_CLOUD=firecracker onctl ls      # expect: unsupported provider "firecracker"
sudo onctl create -n x --provider firecracker   # same — should be rejected
onctl create --help | grep -E 'kernel-image|rootfs-image|fc-binary|vcpu|memory'  # new flags present
```

## Notes

- **`onctl ls` IP column** — double check the column index in step 3; it may
  differ across `onctl` versions.
- **`--username`** now also sets the fc microVM SSH user (defaults to
  `root`); if your rootfs uses a different user, pass `--username`.
