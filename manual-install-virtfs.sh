#!/bin/bash

echo "start: $(date)"

name="test"
## http://cdimage.debian.org/cdimage/openstack/current-9/
variant="debian9"
source_qcow2="debian-9-openstack-amd64.qcow2"

## https://cloud.centos.org/centos/7/images/?C=M;O=D
#source_qcow2="CentOS-7-x86_64-GenericCloud.qcow2"

## https://cloud-images.ubuntu.com/bionic/current/
#source_qcow2="Ubuntu-bionic-server-cloudimg-amd64.qcow2"

## http://cloud-images.ubuntu.com/minimal/releases/bionic/release/
#source_qcow2="ubuntu-18.04-minimal-cloudimg-amd64.qcow2"

function check() {
    err=$?
    if [ $err -ne 0 ]; then
	echo "Error $err, step: $1"
	exit $err
    fi
}

cd $(dirname $0)
sp=$(pwd)

cd /srv/images
check "changing dir"


#genisoimage -output a.iso -volid cidata -joliet -r "$sp/user-data" "$sp/meta-data"
#check "ISO generation"


cp "$source_qcow2" "$name.qcow2"
check "original image copy"

qemu-img resize "$name.qcow2" 20G
check "resize"

# we whould use virsh pools a bit more (or refresh the pool at least ;)

virsh define /home/xfennec/local/KVM/test-virtfs.xml
check "virsh define"

virsh start test
check "virsh start"


echo "end: $(date)"
