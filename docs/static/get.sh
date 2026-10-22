#!/bin/bash
# Installs onctl: downloads the release archive for this OS/arch from GitHub
# Releases, verifies it against the release's checksums.txt, puts the binary
# in $HOME/.local/bin (override with ONCTL_INSTALL_DIR), makes sure that
# directory is on PATH, and sets up shell tab completion.
#
#   curl -fsSL https://onctl.sh/get.sh | bash                        # latest release
#   curl -fsSL https://onctl.sh/get.sh | ONCTL_VERSION=v0.1.34 bash  # a specific tag
#   curl -fsSL https://onctl.sh/get.sh | ONCTL_VERSION=edge bash     # tip of main (get-edge.sh)
#
# Modelled on claude.ai/install.sh: no sudo, everything under $HOME, a
# checksum-verified download, and PATH fixed up in the shell's rc file.

set -euo pipefail

REPO="cdalar/onctl"
GITHUB="https://github.com"
INSTALL_DIR="${ONCTL_INSTALL_DIR:-$HOME/.local/bin}"

# Refuse sudo from a regular user's shell: $HOME would then be root's, so the
# binary would land in /root/.local/bin and 'onctl' would not be found in the
# user's own shell. Plain root (containers, CI) is unaffected.
if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
    echo "Error: do not run this installer with sudo." >&2
    echo "onctl installs into your home directory ($INSTALL_DIR) and does not need root." >&2
    echo "Re-run without sudo:  curl -fsSL https://onctl.sh/get.sh | bash" >&2
    exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
    echo "Error: curl is required but not installed." >&2
    exit 1
fi

# Resolve the release tag. GitHub's /releases/latest skips prereleases, so
# the rolling 'edge' prerelease is only reachable by naming it.
tag="${ONCTL_VERSION:-}"
if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
    tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$tag" ]; then
        echo "Error: could not determine the latest release tag." >&2
        exit 1
    fi
fi

case $(uname -s) in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    CYGWIN*|MINGW*|MSYS*)
        echo "Error: this installer does not support Windows. Download onctl-windows-amd64.zip" >&2
        echo "from $GITHUB/$REPO/releases and put onctl.exe somewhere on your PATH." >&2
        exit 1
        ;;
    *) echo "Error: unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case $(uname -m) in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) echo "Error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

# An x86_64 shell running under Rosetta on an Apple Silicon Mac should still
# get the native arm64 binary.
if [ "$os" = "darwin" ] && [ "$arch" = "amd64" ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = "1" ]; then
    arch="arm64"
fi

# Must match the archive name_template in .goreleaser.yml / .goreleaser.edge.yml
archive="onctl-${os}-${arch}.tar.gz"
base_url="$GITHUB/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading onctl $tag ($os/$arch)"
# -f: an HTTP error (stale or missing asset) fails right here instead of
# saving the error page as if it were the archive.
if ! curl -fsSL "$base_url/$archive" -o "$tmp/$archive"; then
    echo "Error: could not download $base_url/$archive -- is '$tag' a published release?" >&2
    exit 1
fi
curl -fsSL "$base_url/checksums.txt" -o "$tmp/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
if [ -z "$expected" ]; then
    echo "Error: $archive is not listed in checksums.txt for $tag." >&2
    exit 1
fi
if [ "$os" = "darwin" ]; then
    actual=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
else
    actual=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
fi
if [ "$actual" != "$expected" ]; then
    echo "Error: checksum mismatch for $archive." >&2
    echo "  expected $expected" >&2
    echo "  got      $actual" >&2
    exit 1
fi

tar -xzf "$tmp/$archive" -C "$tmp" onctl
mkdir -p "$INSTALL_DIR"
install -m 755 "$tmp/onctl" "$INSTALL_DIR/onctl"
echo "Installed onctl to $INSTALL_DIR/onctl ($("$INSTALL_DIR/onctl" version))"

# Make sure $INSTALL_DIR is on PATH. This script itself always runs under
# bash via `curl | bash`, so $SHELL is the user's login shell, not ours.
shell_name=$(basename "${SHELL:-}")
rc_line="export PATH=\"$INSTALL_DIR:\$PATH\""
case $shell_name in
    bash) rc="$HOME/.bashrc" ;;
    zsh)  rc="$HOME/.zshrc" ;;
    fish) rc="${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish"; rc_line="fish_add_path \"$INSTALL_DIR\"" ;;
    *)    rc="" ;;
esac
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        if [ -n "$rc" ] && grep -qsF "$INSTALL_DIR" "$rc"; then
            echo "$rc already puts $INSTALL_DIR on PATH -- open a new terminal to use onctl."
        elif [ -n "$rc" ]; then
            mkdir -p "$(dirname "$rc")"
            printf '\n# onctl (added by onctl.sh/get.sh)\n%s\n' "$rc_line" >> "$rc"
            echo "Added $INSTALL_DIR to PATH in $rc -- open a new terminal to use onctl."
        else
            echo "Note: $INSTALL_DIR is not on your PATH. Add this to your shell's startup file:"
            echo "    $rc_line"
        fi
        ;;
esac

# Another onctl elsewhere on PATH (an old /usr/local/bin install, a
# `go install` in ~/go/bin) can still be the one that runs: its directory
# may come first, or the running shell may have hashed it before this
# install. Neither is fixable from here -- this script is a child process
# of that shell -- so name the copies and say what to do.
others=$(IFS=:; for d in $PATH; do
    if [ -n "$d" ] && [ -x "$d/onctl" ] && ! [ "$d/onctl" -ef "$INSTALL_DIR/onctl" ]; then
        echo "$d/onctl"
    fi
done | sort -u)
if [ -n "$others" ]; then
    echo "Note: other onctl binaries are on your PATH and may run instead of this one:"
    echo "$others" | sed "s/^/    /"
    echo "Remove them, or run 'hash -r' so your shell stops using a cached path."
fi

# Install shell tab completion (subcommands, flags, VM names for ssh/
# destroy/port-forward, instance sizes) for the shell in $SHELL. The
# completion script is generated by the binary we just installed, so it
# always matches its version. Set ONCTL_NO_COMPLETION=1 to skip this step.
install_completion() {
    local data_dir file rc
    data_dir="${XDG_DATA_HOME:-$HOME/.local/share}"

    case $shell_name in
        bash)
            # bash-completion v2's per-user directory, lazy-loaded on the
            # first <TAB> after `onctl`...
            file="$data_dir/bash-completion/completions/onctl"
            rc="$HOME/.bashrc"
            ;;
        zsh)
            file="$data_dir/zsh/site-functions/_onctl"
            rc="$HOME/.zshrc"
            ;;
        fish)
            # fish autoloads this directory; no rc change needed.
            file="${XDG_CONFIG_HOME:-$HOME/.config}/fish/completions/onctl.fish"
            rc=""
            ;;
        *)
            echo "Skipping shell completion: unrecognized shell '${shell_name:-unknown}'."
            echo "Run 'onctl completion --help' to set it up by hand."
            return 0
            ;;
    esac

    mkdir -p "$(dirname "$file")"
    # -s: releases before the completion command stopped needing an
    # initialized .onctl directory exit 0 here without printing a script.
    if ! "$INSTALL_DIR/onctl" completion "$shell_name" > "$file" || [ ! -s "$file" ]; then
        echo "Warning: could not generate $shell_name completion; skipping."
        rm -f "$file"
        return 0
    fi
    echo "Installed $shell_name completion to $file"

    # ...but plenty of setups (no bash-completion package, oh-my-bash, a
    # zsh without that directory on its fpath) never look there, so also
    # source it from the rc file -- once, and harmless if both happen.
    # zsh needs compinit to have run first (compdef must exist), which is
    # why the line goes at the end of .zshrc and checks for it.
    if [ -n "$rc" ] && ! grep -qsF "$file" "$rc"; then
        {
            echo ""
            echo "# onctl shell completion (added by onctl.sh/get.sh)"
            if [ "$shell_name" = "zsh" ]; then
                echo "(( \$+functions[compdef] )) && [ -r \"$file\" ] && source \"$file\""
            else
                echo "[ -r \"$file\" ] && source \"$file\""
            fi
        } >> "$rc"
        echo "Added a line to $rc to load it -- open a new terminal to use it."
    fi
}

if [ -z "${ONCTL_NO_COMPLETION:-}" ]; then
    install_completion
fi

echo ""
echo "Installation complete. Run 'onctl init' to get started."
