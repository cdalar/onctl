#!/bin/bash

# Repository details
REPO="cdalar/onctl"
GITHUB="https://github.com"

# Get the latest release tag from GitHub API
latest_release=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

# Check if we got a valid release tag
if [ -z "$latest_release" ]; then
    echo "Error: Could not get latest release tag. Exiting."
    exit 1
fi

echo "Latest release tag is: $latest_release"

# Determine system architecture
architecture=$(uname -m)
case $architecture in
    x86_64)
        arch="amd64"
        ;;
    arm64)
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
        ;;
    Darwin)
        os="darwin"
        ;;
    CYGWIN*|MINGW32*|MSYS*|MINGW*)
        os="windows"
        ;;
    *)
        echo "Error: Unsupported operating system: $os"
        exit 1
        ;;
esac

echo "Operating system is: $os"
echo "System architecture is: $architecture"

# Construct download URL
# This URL pattern is an example and needs to match the actual pattern used in the releases
download_url="$GITHUB/$REPO/releases/download/$latest_release/onctl-${os}-${arch}.tar.gz"

# Download the binary
echo "Downloading parampiper from $download_url"
curl -L $download_url -o "onctl-${os}-${arch}-${latest_release}.tar.gz"

tar zxvf "onctl-${os}-${arch}-${latest_release}.tar.gz" onctl

echo "Download complete. onctl binary is in the current directory."
