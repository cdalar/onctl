#!/bin/bash
apt update
apt install -y wireguard qrencode nftables nginx
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv6.conf.all.forwarding=1
wg genkey | tee /etc/wireguard/private.key
wg genkey | tee /etc/wireguard/private-client.key
chmod go= /etc/wireguard/private.key
cat /etc/wireguard/private.key | wg pubkey | tee /etc/wireguard/public.key
cat /etc/wireguard/private-client.key | wg pubkey | tee /etc/wireguard/public-client.key
cat > /etc/wireguard/wg0.conf <<EOL
# define the WireGuard service
[Interface]
# contents of file wg-private.key that was recently created
PrivateKey = $(cat /etc/wireguard/private.key)

# UDP service port; 51820 is a common choice for WireGuard
ListenPort = 51820
PostUp = nft add table ip wireguard; nft add chain ip wireguard wireguard_chain {type nat hook postrouting priority srcnat\; policy accept\;}; nft add rule ip wireguard wireguard_chain counter packets 0 bytes 0 masquerade; nft add table ip6 wireguard; nft add chain ip6 wireguard wireguard_chain {type nat hook postrouting priority srcnat\; policy accept\;}; nft add rule ip6 wireguard wireguard_chain counter packets 0 bytes 0 masquerade
PostDown = nft delete table ip wireguard; nft delete table ip6 wireguard

# define the remote WireGuard interface (client)
[Peer]

# contents of wg-public-client.key
PublicKey = $(cat /etc/wireguard/public-client.key)

# the IP address of the client on the WireGuard network
AllowedIPs = 192.168.2.0/24
EOL

#wg setconf wg0 /etc/wireguard/wg0.conf
#ip link set up dev wg0

# WireGuard Client Config
cat > ~/wg-client.conf <<EOL2
# define the local WireGuard interface (client)
[Interface]

# contents of wg-private-client.key
PrivateKey = $(cat /etc/wireguard/private-client.key)

# the IP address of this client on the WireGuard network
Address=192.168.2.2/32

DNS=8.8.8.8

# define the remote WireGuard interface (server)
[Peer]

# from `sudo wg show wg0 public-key`
PublicKey = $(cat /etc/wireguard/public.key)

# the IP address of the server on the WireGuard network
#AllowedIPs = 10.0.2.1/32
AllowedIPs = 0.0.0.0/0, ::/0

# public IP address and port of the WireGuard server
Endpoint = $(ifconfig eth0 | grep 'inet ' | cut -d ' ' -f 10):51820

PersistentKeepalive = 25
EOL2

wg-quick up wg0
wg show

# Put client config in web server root
qrencode -t png -o /var/www/html/qr.png < ~/wg-client.conf
cp ~/wg-client.conf /var/www/html/wg-client.conf