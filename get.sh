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
        os="darwin"
        extension=".tar.gz"
        unzip_command="tar zxvf"
        ;;
    CYGWIN*|MINGW32*|MSYS*|MINGW*)
        os="windows"
        extension=".zip"
        unzip_command="unzip -o"
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
download_url="$GITHUB/$REPO/releases/download/$latest_release/onctl-${os}-${arch}${extension}"

# Download the binary
echo "Downloading parampiper from $download_url"
curl -L $download_url -o "onctl-${os}-${arch}-${latest_release}${extension}"

# Unzip the binary if on Windows or use tar command if on Linux
if [ "$os" = "windows" ]; then
    echo "Unzipping onctl-${os}-${arch}-${latest_release}${extension}"
    $unzip_command "onctl-${os}-${arch}-${latest_release}${extension}" onctl.exe
else
    echo "Extracting onctl-${os}-${arch}-${latest_release}${extension}"
    $unzip_command "onctl-${os}-${arch}-${latest_release}${extension}" onctl
fi

echo "Download and unzip complete. onctl binary is in the current directory."
