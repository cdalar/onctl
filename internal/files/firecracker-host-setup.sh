#!/bin/bash
set -ex

# Sets up a remote VM (e.g. a GCP instance with nested virtualization enabled)
# to run Firecracker microVMs via `ONCTL_CLOUD=firecracker onctl ...`.

apt-get update
apt-get install -y curl iproute2 e2fsprogs

if [ -e /dev/kvm ]; then
  echo "/dev/kvm is available"
else
  echo "WARNING: /dev/kvm not found. Enable nested virtualization on this VM" \
       "(e.g. set gcp.vm.nestedVirtualization: true) before running Firecracker."
fi

ARCH="$(uname -m)"

# Install the firecracker binary.
release_url="https://github.com/firecracker-microvm/firecracker/releases"
latest_version=$(basename "$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${release_url}/latest")")
curl -fsSL "${release_url}/download/${latest_version}/firecracker-${latest_version}-${ARCH}.tgz" | tar -xz
mv "release-${latest_version}-${ARCH}/firecracker-${latest_version}-${ARCH}" /usr/local/bin/firecracker
chmod +x /usr/local/bin/firecracker
rm -rf "release-${latest_version}-${ARCH}"

# Download a sample kernel and rootfs image for quick microVM testing.
mkdir -p ~/.onctl/firecracker/images
cd ~/.onctl/firecracker/images

CI_VERSION="v1.10"
kernel_key=$(curl -fsSL "https://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-5.10&list-type=2" \
  | grep -oP "(?<=<Key>)(firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-5\.10\.[0-9]+)(?=</Key>)" \
  | sort -V | tail -1)
curl -fsSL "https://s3.amazonaws.com/spec.ccfc.min/${kernel_key}" -o vmlinux

rootfs_key=$(curl -fsSL "https://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-24.04.ext4&list-type=2" \
  | grep -oP "(?<=<Key>)(firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-24\.04\.ext4)(?=</Key>)" \
  | sort -V | tail -1)
curl -fsSL "https://s3.amazonaws.com/spec.ccfc.min/${rootfs_key}" -o rootfs.ext4

# Install onctl itself so microVMs can be managed from this host.
curl -sLS https://docs.onctl.io/get.sh | bash
install onctl /usr/local/bin/

echo "Firecracker host setup complete."
echo "Run: ONCTL_CLOUD=firecracker onctl init"
echo "Then: ONCTL_CLOUD=firecracker onctl create -n my-microvm"
