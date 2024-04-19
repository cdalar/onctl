#!/bin/bash

# Repository details
REPO="cdalar/onctl"
GITHUB="https://github.com"

# Construct download URL
# This URL pattern is an example and needs to match the actual pattern used in the releases
download_url="$GITHUB/$REPO/releases/download/v0.1.0/onctl-linux"

# Download the binary
curl -L $download_url -o "onctl"
chmod +x onctl
