#!/usr/bin/env bash
# Test stub for `gh`: records its arguments to $STUB_LOG.gh and prints a
# fake JIT config blob, mimicking `gh api ... -q .encoded_jit_config`.
set -euo pipefail
echo "$@" >>"${STUB_LOG}.gh"
echo "fake-jit-config-blob"
