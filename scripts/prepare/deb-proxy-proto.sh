#!/bin/bash

# -- Run with sudo privileges
# For: Debian 11+ / Ubuntu 20.04+

# This script will install a "PROXY protocol" server to allow TCP
# port redirections (see "ports" option in TOML config files) to
# get the real client IP address.

# No configuration is required, just add "PROXY" comment in your port
# definition in the TOML config file:
# "22/tcp->@PUBLIC:2222 (PROXY)",

export DEBIAN_FRONTEND="noninteractive"

sudo -E apt-get -y -qq install golang git || exit $?

git clone https://github.com/Xfennec/go-mmproxy.git || exit $?

cd go-mmproxy || exit $?
go install || exit $?
cd .. || exit $?


# go-mmproxy spoofs the client source address: the replies must be routed
# back to the loopback. systemd-networkd has to own this rule, or it will
# flush it as "foreign" on its next start (netplan does not manage lo).
sudo bash -c "cat > /etc/systemd/network/10-mmproxy.network" <<- 'EOS'
# added by deb-proxy-proto.sh
[Match]
Name=lo

[RoutingPolicyRule]
From=127.0.0.1/8
IncomingInterface=lo
Table=123

[Route]
Type=local
Destination=0.0.0.0/0
Table=123
EOS
[ $? -eq 0 ] || exit $?

sudo networkctl reload || exit $?
sudo networkctl reconfigure lo || exit $?


sudo bash -c "cat > /etc/systemd/system/mmproxy.service" <<- EOS
[Unit]
Description=mmproxy
After=network.target systemd-networkd.service

[Service]
Type=simple
LimitNOFILE=65535

ExecStart=$HOME/go/bin/go-mmproxy -dynamic-destination

Restart=on-failure
RestartSec=3s

## https://www.freedesktop.org/software/systemd/man/systemd.exec.html#Capabilities
#AmbientCapabilities=CAP_NET_ADMIN
# CAP_NET_RAW CAP_NET_BIND_SERVICE
User=root

NoNewPrivileges=true
PrivateDevices=true
PrivateTmp=true
ProtectSystem=full
ProtectKernelTunables=true

[Install]
WantedBy=multi-user.target
EOS
[ $? -eq 0 ] || exit $?

sudo systemctl daemon-reload || exit $?

sudo systemctl enable --now mmproxy || exit $?
