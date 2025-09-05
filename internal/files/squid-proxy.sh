#!/bin/bash
set -ex

# Install Squid proxy server
apt-get update
apt-get install -y squid

# Configure Squid to allow connections from private network
cat > /etc/squid/squid.conf << EOF
# Squid configuration for jump host proxy
http_port 3128

# Allow access from private network (adjust CIDR as needed)
acl private_network src 172.31.0.0/16
http_access allow private_network

# Allow localhost
acl localhost src 127.0.0.1/32
http_access allow localhost

# Deny all other access
http_access deny all

# Basic settings
cache_mem 256 MB
maximum_object_size 128 MB
cache_dir ufs /var/spool/squid 1000 16 256

# Log settings
access_log /var/log/squid/access.log
cache_log /var/log/squid/cache.log

# DNS settings
dns_nameservers 8.8.8.8 8.8.4.4
EOF

# Create cache directory
squid -z

# Start and enable Squid
systemctl enable squid
systemctl start squid

# Configure firewall to allow proxy traffic
ufw allow 3128/tcp

echo "Squid proxy server installed and configured on port 3128"
echo "Private network (172.31.0.0/16) can now use this host as a proxy"

