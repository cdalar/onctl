#!/bin/bash
set -ex

# Configure DNS to use public DNS servers
# This allows the VM to resolve hostnames even without direct internet access

# Backup original resolv.conf
cp /etc/resolv.conf /etc/resolv.conf.backup

# Create a static resolv.conf with public DNS servers
cat > /etc/resolv.conf << EOF
# Static DNS configuration for proxy access
nameserver 8.8.8.8
nameserver 8.8.4.4
nameserver 1.1.1.1
search .
EOF

# Test DNS resolution
echo "Testing DNS resolution..."
nslookup google.com
nslookup download.docker.com

echo "DNS configuration updated. Using public DNS servers: 8.8.8.8, 8.8.4.4, 1.1.1.1"

