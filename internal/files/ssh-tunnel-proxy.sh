#!/bin/bash
set -ex

# Set up SSH tunnel proxy
JUMPHOST_IP="${JUMPHOST_IP:-172.31.37.136}"
JUMPHOST_USER="${JUMPHOST_USER:-ubuntu}"
LOCAL_PROXY_PORT="${LOCAL_PROXY_PORT:-1080}"

# Create a background SSH tunnel that acts as a SOCKS proxy
ssh -f -N -D ${LOCAL_PROXY_PORT} ${JUMPHOST_USER}@${JUMPHOST_IP}

# Configure environment variables to use the SOCKS proxy
export http_proxy="socks5://127.0.0.1:${LOCAL_PROXY_PORT}"
export https_proxy="socks5://127.0.0.1:${LOCAL_PROXY_PORT}"
export HTTP_PROXY="socks5://127.0.0.1:${LOCAL_PROXY_PORT}"
export HTTPS_PROXY="socks5://127.0.0.1:${LOCAL_PROXY_PORT}"

# Configure apt to use SOCKS proxy (requires apt-transport-socks5)
apt-get update
apt-get install -y apt-transport-socks5

cat > /etc/apt/apt.conf.d/socks-proxy.conf << EOF
Acquire::http::Proxy "socks5://127.0.0.1:${LOCAL_PROXY_PORT}";
Acquire::https::Proxy "socks5://127.0.0.1:${LOCAL_PROXY_PORT}";
EOF

echo "SSH tunnel proxy configured on port ${LOCAL_PROXY_PORT}"
echo "Using SOCKS5 proxy through jump host ${JUMPHOST_IP}"

