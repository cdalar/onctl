#!/usr/bin/env bash
# Phase 1 spike: bootstrap a GitHub Actions runner via JIT config on a fresh
# Ubuntu VM. Designed to run as root via `onctl create -a`.
#
# Required vars (pass with -e):
#   JIT_CONFIG    base64 JIT config blob, generate with:
#                 gh api -X POST repos/$GH_REPO/actions/runners/generate-jitconfig \
#                   -f name=runner-spike-jit -F runner_group_id=1 \
#                   -f 'labels[]=self-hosted' -f 'labels[]=onctl' \
#                   -q .encoded_jit_config
# Optional vars:
#   SKIP_DOCKER   set to 1 to skip docker install (faster boot measurement)
set -euo pipefail

T0=$(date +%s)
say() { echo "[github-runner-jit +$(($(date +%s) - T0))s] $*"; }

JIT_CONFIG="${JIT_CONFIG:?JIT_CONFIG is required, pass with -e JIT_CONFIG=...}"
RUNNER_USER=runner
RUNNER_HOME=/opt/actions-runner

say "installing packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq curl jq git tar >/dev/null

if [ "${SKIP_DOCKER:-0}" != "1" ]; then
    say "installing docker"
    curl -fsSL https://get.docker.com | sh >/dev/null 2>&1
fi

say "creating ${RUNNER_USER} user"
if ! id "$RUNNER_USER" >/dev/null 2>&1; then
    useradd -m -s /bin/bash "$RUNNER_USER"
fi
getent group docker >/dev/null 2>&1 && usermod -aG docker "$RUNNER_USER"

say "downloading actions-runner"
case "$(uname -m)" in
    x86_64) ARCH=x64 ;;
    aarch64) ARCH=arm64 ;;
    *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac
VERSION=$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest | jq -r '.tag_name | ltrimstr("v")')
mkdir -p "$RUNNER_HOME"
curl -fsSL "https://github.com/actions/runner/releases/download/v${VERSION}/actions-runner-linux-${ARCH}-${VERSION}.tar.gz" \
    | tar xz -C "$RUNNER_HOME"
chown -R "$RUNNER_USER:$RUNNER_USER" "$RUNNER_HOME"

say "starting runner via JIT config in background (ephemeral: exits after one job)"
cd "$RUNNER_HOME"
sudo -u "$RUNNER_USER" bash -c "nohup ./run.sh --jitconfig '$JIT_CONFIG' > '$RUNNER_HOME/jit-run.log' 2>&1 &"

say "runner launched, no service installed"
