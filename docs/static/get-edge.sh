#!/bin/bash
# Installs the "edge" build of onctl: a snapshot built from the tip of main
# on every push (see .github/workflows/edge.yml), published as a single
# rolling prerelease at a fixed tag rather than a versioned one. Linux and
# macOS only -- the edge workflow does not build Windows binaries.
#
# Same installer as get.sh, pinned to that tag.
set -euo pipefail
curl -fsSL https://onctl.sh/get.sh | ONCTL_VERSION=edge bash
