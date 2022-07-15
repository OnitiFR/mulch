#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9+ / Ubuntu 18.04+

# You import port 2049 from a NFS server, ex:
# ports = [
#    "2049/tcp<-@group",
# ]

# You must also define NFS4_MOUNT, the path where
# the server root will be mounted (directory will
# be created by the script)


if [ -z "$NFS4_MOUNT" ]; then
    >&2 echo "need NFS4_MOUNT env var"
    exit 1
fi

if [ -z "$_2049_TCP" ]; then
    >&2 echo "you must import port tcp/2049 from a NFS server"
    exit 1
fi

sudo mkdir -p "$NFS4_MOUNT" || exit $?
sudo chown "$_APP_USER:$_APP_USER" "$NFS4_MOUNT" || exit $?

export DEBIAN_FRONTEND="noninteractive"

sudo apt-get -y -qq install nfs-common || exit $?

sudo bash -c "cat >> /etc/fstab" <<- EOS
# (added by deb-nfs4-client.sh)
$_MULCH_PROXY_IP:/	$NFS4_MOUNT	nfs4	proto=tcp,port=$_2049_TCP 0 2
EOS
[ $? -eq 0 ] || exit $?

echo "mounting NFS filesystems"
sudo mount -a || exit $?
