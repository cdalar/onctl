#!/usr/bin/env bash
# Installs or updates onctl-dev (onctl built from the main branch) via Homebrew.
set -euo pipefail

brew install --HEAD --fetch-HEAD cdalar/tap/onctl-dev
