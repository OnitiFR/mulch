#!/bin/bash

# TODO: look dynamically for this disk
part="/dev/vdb"

# TODO: check if we should sleep so the system detects the new disk?

sudo mkfs.ext2 "$part" || exit $?
sudo mkdir -p /mnt/backup || exit $?
sudo mount "$part" /mnt/backup || exit $?
