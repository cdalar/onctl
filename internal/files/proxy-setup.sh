#!/bin/bash
set -ex

# Configure HTTP proxy to use jump host
JUMPHOST_IP="${JUMPHOST_IP:-10.0.0.2}"  # Default to jump host private IP for Hetzner
PROXY_PORT="${PROXY_PORT:-3128}"  # Default proxy port

# Set HTTP proxy environment variables
export http_proxy="http://${JUMPHOST_IP}:${PROXY_PORT}"
export https_proxy="http://${JUMPHOST_IP}:${PROXY_PORT}"
export HTTP_PROXY="http://${JUMPHOST_IP}:${PROXY_PORT}"
export HTTPS_PROXY="http://${JUMPHOST_IP}:${PROXY_PORT}"

# Configure apt to use proxy
cat > /etc/apt/apt.conf.d/proxy.conf << EOF
Acquire::http::Proxy "http://${JUMPHOST_IP}:${PROXY_PORT}";
Acquire::https::Proxy "http://${JUMPHOST_IP}:${PROXY_PORT}";
EOF

# Configure wget to use proxy
cat > /etc/wgetrc << EOF
http_proxy = http://${JUMPHOST_IP}:${PROXY_PORT}
https_proxy = http://${JUMPHOST_IP}:${PROXY_PORT}
use_proxy = on
EOF

# Configure curl to use proxy
cat > /etc/curlrc << EOF
proxy = ${JUMPHOST_IP}:${PROXY_PORT}
EOF

echo "Proxy configuration completed. Using jump host ${JUMPHOST_IP}:${PROXY_PORT} for internet access."
