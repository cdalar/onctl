#!/usr/bin/env bash
# Test stub for `onctl`: records its arguments to $STUB_LOG.onctl.
set -euo pipefail
echo "$@" >>"${STUB_LOG}.onctl"
