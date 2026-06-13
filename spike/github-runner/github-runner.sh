#!/usr/bin/env bash
# Phase 0 spike: bootstrap an ephemeral GitHub Actions runner on a fresh
# Ubuntu VM. Designed to run as root via `onctl create -a`.
#
# Required vars (pass with -e):
#   GH_REPO       owner/repo to register against
#   RUNNER_TOKEN  registration token (gh api -X POST repos/$GH_REPO/actions/runners/registration-token -q .token)
# Optional vars:
#   RUNNER_LABELS  extra labels, comma-separated (default: onctl)
#   SKIP_DOCKER    set to 1 to skip docker install (faster boot measurement)
set -euo pipefail

T0=$(date +%s)
say() { echo "[github-runner +$(($(date +%s) - T0))s] $*"; }

GH_REPO="${GH_REPO:?GH_REPO (owner/repo) is required, pass with -e GH_REPO=owner/repo}"
RUNNER_TOKEN="${RUNNER_TOKEN:?RUNNER_TOKEN is required, pass with -e RUNNER_TOKEN=...}"
RUNNER_LABELS="${RUNNER_LABELS:-onctl}"
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

say "registering ephemeral runner for ${GH_REPO}"
sudo -u "$RUNNER_USER" "$RUNNER_HOME/config.sh" \
    --url "https://github.com/${GH_REPO}" \
    --token "$RUNNER_TOKEN" \
    --name "$(hostname)" \
    --labels "$RUNNER_LABELS" \
    --ephemeral \
    --unattended

say "installing and starting runner service"
cd "$RUNNER_HOME"
./svc.sh install "$RUNNER_USER" >/dev/null
./svc.sh start >/dev/null

say "runner online, waiting for a job (ephemeral: exits after one job)"
