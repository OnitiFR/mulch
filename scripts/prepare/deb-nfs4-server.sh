#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9+ (Ubuntu 22.04 will probably fail)

if [ -z "$NFS4_EXPORT" ]; then
    >&2 echo "need NFS4_EXPORT env var (exported path)"
    exit 1
fi

export DEBIAN_FRONTEND="noninteractive"

sudo apt-get -y -qq install nfs-kernel-server || exit $?

app_uid=$(id -u $_APP_USER) || exit $?
app_gid=$(id -g $_APP_USER) || exit $?

sudo mkdir -p "$NFS4_EXPORT" || exit $?
sudo chown "$_APP_USER:$_APP_USER" "$NFS4_EXPORT" || exit $?

sudo bash -c "cat >> /etc/exports" <<- EOS

# (added by deb-nfs4-server.sh)
# exportfs -arv
$NFS4_EXPORT *(rw,sync,fsid=0,crossmnt,no_subtree_check,sec=sys,insecure,anonuid=$app_uid,anongid=$app_gid)
EOS
[ $? -eq 0 ] || exit $?

sudo bash -c "cat >> /etc/default/nfs-kernel-server" <<- 'EOS'

# (this overload was created by deb-nfs4-server.sh)
RPCMOUNTDOPTS="--no-nfs-version 2 --no-nfs-version 3 --nfs-version 4 --no-udp"
RPCNFSDOPTS="--no-nfs-version 2 --no-nfs-version 3 --nfs-version 4 --no-udp"
EOS
[ $? -eq 0 ] || exit $?

sudo systemctl disable --now rpcbind.service rpcbind.socket || exit $?
sudo systemctl mask rpcbind.service rpcbind.socket || exit $?

sudo systemctl restart nfs-server || exit $?