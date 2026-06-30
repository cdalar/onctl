#!/usr/bin/env bash
set -euo pipefail

REMOTE_PATH="/usr/local/bin/onctl"

echo "Building onctl for linux/amd64..."
make build-amd64

echo "Uploading to onctl-controller:/tmp/onctl..."
./onctl ssh onctl-controller --upload "onctl-amd64:/tmp/onctl"

echo "Installing binary..."
./onctl ssh onctl-controller -- "install -m 755 /tmp/onctl ${REMOTE_PATH} && rm /tmp/onctl"

echo "Done. Verifying:"
./onctl ssh onctl-controller -- "onctl version"
