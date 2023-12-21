#!/bin/sh
echo "Port 53" >> /etc/ssh/sshd_config
service ssh restart
