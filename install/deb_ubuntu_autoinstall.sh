#!/bin/bash

# Note: only Ubuntu 22.04+ is supported currently, since Go 1.18+ is needed

# Debian 10: git, pkg-config, build-essential, qemu-kvm

echo "This is a Mulch server install script for Ubuntu."
echo "It was tested on: (default server install)"
echo " - Ubuntu 22.04"
#echo " - Debian 10"
echo "It's intended to be used for a quick demo install, since most settings are left to default values."
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

# 1st line is Debian (10) specific
apt update || exit $?
apt -y -qq install \
    git pkg-config build-essential qemu-kvm \
    golang-go \
    ebtables gawk libxml2-utils libcap2-bin dnsmasq \
    libvirt-daemon-system libvirt-dev \
    || exit $?

if [ ! -d /home/mulch ]; then
    useradd mulch -s /bin/bash -m -G libvirt || exit $?
fi

setfacl -m g:libvirt-qemu:x /home/mulch/ || exit $?

#echo "Updating Go ACME client… (needed for Let's Encrypt ACMEv2)"
#go get -u golang.org/x/crypto/acme/autocert || exit $?

echo "Compiling and installing mulch…"
sudo -iu mulch mkdir -p /home/mulch/go/src/github.com/OnitiFR/mulch || exit $?
sudo -iu mulch git clone https://github.com/OnitiFR/mulch.git /home/mulch/go/src/github.com/OnitiFR/mulch || exit $?
sudo -iu mulch sh -c "cd /home/mulch/go/src/github.com/OnitiFR/mulch && go install ./cmd/..." || exit $?
sudo -iu mulch mkdir -p /home/mulch/mulch/etc /home/mulch/mulch/data /home/mulch/mulch/storage || exit $?
cd /home/mulch/go/src/github.com/OnitiFR/mulch || exit $?
sudo -iu mulch /home/mulch/go/src/github.com/OnitiFR/mulch/install.sh --etc /home/mulch/mulch/etc/ --data /home/mulch/mulch/data/ --storage /home/mulch/mulch/storage/ || exit $?
echo "-- OK, let me do most of this setup for you…"

setcap 'cap_net_bind_service=+ep' /home/mulch/go/bin/mulch-proxy || exit $?
cp mulchd.service mulch-proxy.service /etc/systemd/system/ || exit $?
systemctl daemon-reload || exit $?

model=$(virsh capabilities | xmllint  --xpath 'string(/capabilities/host/cpu/model)' -)
if [ $? -ne 0 ]; then
    echo "Error detecting CPU capabilities"
    exit $?
fi
sudo -iu mulch sed -i'' "s|<model fallback='allow'>.*</model>|<model fallback='allow'>$model</model>|" /home/mulch/mulch/etc/templates/vm.xml || exit $?

sudo -iu mulch sed -i'' "s|^proxy_acme_email =.*|proxy_acme_email = \"mulch-testing@oniti.fr\"|" /home/mulch/mulch/etc/mulchd.toml

echo "Enabling and testing services…"
systemctl enable --now mulchd || exit $?

db="/home/mulch/mulch/data/mulch-proxy-domains.db"
echo "Waiting for mulch-proxy-domains.db…"
while [ ! -f $db ]; do
  sleep 1
done

sleep 3
systemctl enable --now mulch-proxy || exit $?
sleep 3

systemctl is-active --quiet mulchd
if [ $? -ne 0 ]; then
    echo "Error, see systemctl status mulchd"
    exit $?
fi

systemctl is-active --quiet mulch-proxy
if [ $? -ne 0 ]; then
    echo "Error, see systemctl status mulch-proxy"
    exit $?
fi

echo "Installation completed."

db="/home/mulch/mulch/data/mulch-api-keys.db"
echo "Waiting for your API key…"
while [ ! -f $db ]; do
  sleep 1
done
echo "Your API key is:"
grep Key $db

echo ""
echo "Sample ~/.toml file:"
echo ""
echo '[[server]]'
echo 'name = "demo"'
echo 'url = "http://xxxxx:8686"'
echo 'key = "xxxxx"'
echo ""
echo "See also https://letsencrypt.org/docs/staging-environment/ to add the 'fake LE root' certificate to your browser for HTTPS tests (and change proxy_acme_email with your own email address)"
