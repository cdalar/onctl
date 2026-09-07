#!/bin/bash

# Installs the "edge" build of onctl: an unsigned snapshot built from the
# tip of main on every push (see .github/workflows/edge.yml), published as
# a single rolling prerelease at a fixed tag rather than a versioned one.
# Linux only -- the edge workflow does not build macOS or Windows binaries.

set -euo pipefail

# Repository details
REPO="cdalar/onctl"
GITHUB="https://github.com"
TAG="edge"

# Determine system architecture
architecture=$(uname -m)
case $architecture in
    x86_64)
        arch="amd64"
        ;;
    arm64*|aarch64*)
        arch="arm64"
        ;;
    *)
        echo "Error: Unsupported architecture: $architecture"
        exit 1
        ;;
esac

# Determine operating system
os=$(uname -s)
case $os in
    Linux)
        os="linux"
        extension=".tar.gz"
        unzip_command="tar zxvf"
        ;;
    Darwin)
        echo "Error: no edge build is published for macOS -- use 'brew install --HEAD --fetch-HEAD cdalar/tap/onctl-dev' instead."
        exit 1
        ;;
    CYGWIN*|MINGW32*|MSYS*|MINGW*)
        echo "Error: no edge build is published for Windows -- download a tagged release from the releases page instead."
        exit 1
        ;;
    *)
        echo "Error: Unsupported operating system: $os"
        exit 1
        ;;
esac

echo "Installing onctl edge build ($os/$arch) from the '$TAG' release"

# Construct download URL -- must match .goreleaser.edge.yml's archive name_template
download_url="$GITHUB/$REPO/releases/download/$TAG/onctl-${os}-${arch}${extension}"
archive="onctl-${os}-${arch}-${TAG}${extension}"

# Download the binary. --fail makes curl exit non-zero on an HTTP error
# response (e.g. a stale/missing asset) instead of saving the error page as
# if it were the archive; set -e above then stops the script right here
# instead of it reaching the "complete" message with no usable binary.
echo "Downloading onctl from $download_url"
curl -fL "$download_url" -o "$archive"

echo "Extracting $archive"
$unzip_command "$archive" onctl

echo "Download and unzip complete. onctl binary is in the current directory."
