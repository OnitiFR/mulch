#!/bin/bash

if [ "$#" -ne 3 ]; then
    echo "This script will create an empty QCOW2 image for a backup file and mount it."
    echo "It may be needed for testing."
    echo "- Usage: $0 disk size mountpoint"
    echo "- Example: $0 test.qcow2 2G /mnt/test"
    exit 1
fi

disk_name="$1"
disk_size="$2"
mount_point="$3"

if [ $(id -u) != "0" ]; then
    echo "Must be run as root."
fi

modprobe nbd

qemu-img create -f qcow2 "$disk_name" "$disk_size" || exit $?
qemu-nbd -c /dev/nbd0 "$disk_name" || exit $?
mke2fs -L backup /dev/nbd0 || exit $?
mount /dev/nbd0 "$3" || exit $?
chmod 777 "$3" || exit $?

echo "Image $1 created and mounted: $3"
echo -n "When ready, press enter to unmount…"
read

umount /dev/nbd0 || exit $?
qemu-nbd -d /dev/nbd0 || exit $?

echo "Unmounted. Image is ready."
echo "You can transparently compress it using:"
echo "qemu-img convert -O qcow2 -c $1 ${1}c"
