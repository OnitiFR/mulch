#!/bin/bash

echo "This is a Mulch server install script for Ubuntu."
echo "It was tested on 18.04 to 19.04, witch default server install."
echo "It's intended to be used for a quick demo install, since most settings are left to their default."
echo ""
echo "This script will install packages, services, create users, etc. IT MAY BUTCHER YOUR SYSTEM!"
echo "So use it on a blank test server, not on an \"important\" system."
echo ""
echo "Press CTRL+c to cancel, or Enter to continue."

read
echo "OK, starting install…"

uid=$(id -u)
if [ "$uid" -ne 0 ]; then
    echo "Error: this script must be ran as root (ex: sudo -i)"
    exit 1
fi

apt -y -qq install golang-go || exit $?
apt -y -qq install ebtables gawk libxml2-utils libcap2-bin dnsmasq libvirt-daemon-system libvirt-dev || exit $?
useradd mulch -s /bin/bash -m -G libvirt || exit $?

echo "Compiling and installing mulch…"
sudo -iu mulch go get -u github.com/OnitiFR/mulch/cmd/... || exit $?
sudo -iu mulch mkdir -p /home/mulch/mulch/etc /home/mulch/mulch/data /home/mulch/mulch/storage || exit $?
cd /home/mulch/go/src/github.com/OnitiFR/mulch || exit $?
sudo -iu mulch /home/mulch/go/src/github.com/OnitiFR/mulch/install.sh --etc /home/mulch/mulch/etc/ --data /home/mulch/mulch/data/ --storage /home/mulch/mulch/storage/ || exit $?

setcap 'cap_net_bind_service=+ep' /home/mulch/go/bin/mulch-proxy || exit $?
cp mulchd.service mulch-proxy.service /etc/systemd/system/ || exit $?
systemctl daemon-reload || exit $?
systemctl enable --now mulchd || exit $?
systemctl enable --now mulch-proxy || exit $?
echo "Testing services…"
sleep 5
systemctl is-active --quiet mulchd || (echo "Error, see systemctl status mulchd" ; exit $?)
systemctl is-active --quiet mulch-proxy || (echo "Error, see systemctl status mulch-proxy" ; exit $?)

echo "Installation completed."

echo "Your API key:"
grep Key /home/mulch/mulch/data/mulch-api-keys.db

echo ""
echo "Sample ~/.toml file:"
echo ""
echo '[[server]]'
echo 'name = "demo"'
echo 'url = "http://xxxxx:8585"'
echo 'key = "xxxxx"'
