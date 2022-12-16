#!/bin/bash

# -- Run with sudo privileges
# For: CentOS 7

# This script will install a "PROXY protocol" server to allow TCP
# port redirections (see "ports" option in TOML config files) to
# get the real client IP address.

# No configuration is required, just add "PROXY" comment in your port
# definition in the TOML config file:
# "22/tcp->@PUBLIC:2222 (PROXY)",

sudo rpm --import https://mirror.go-repo.io/centos/RPM-GPG-KEY-GO-REPO || exit $?
curl -s https://mirror.go-repo.io/centos/go-repo.repo | sudo tee /etc/yum.repos.d/go-repo.repo || exit $?
sudo yum -y install golang git || exit $?

git clone https://github.com/Xfennec/go-mmproxy.git || exit $?

cd go-mmproxy || exit $?
go install || exit $?
cd .. || exit $?


sudo bash -c "cat > /etc/systemd/system/mmproxy.service" <<- EOS
[Unit]
Description=mmproxy
After=network.target

[Service]
Type=simple
LimitNOFILE=65535
ExecStartPost=/sbin/ip rule add from 127.0.0.1/8 iif lo table 123
ExecStartPost=/sbin/ip route add local 0.0.0.0/0 dev lo table 123

ExecStart=$HOME/go/bin/go-mmproxy -dynamic-destination

ExecStopPost=/sbin/ip rule del from 127.0.0.1/8 iif lo table 123
ExecStopPost=/sbin/ip route del local 0.0.0.0/0 dev lo table 123

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
