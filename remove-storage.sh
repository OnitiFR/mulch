#!/bin/bash

# Crude script to remove all Mulch storage pool definitions, for test purposes
# It's supposed to connect to qemu:///system [not session], so
# run as root to make your life simplier)

virsh pool-destroy mulch-seeds
virsh pool-destroy mulch-disks
virsh pool-destroy mulch-cloud-init
virsh pool-destroy mulch-backups

virsh pool-undefine mulch-seeds
virsh pool-undefine mulch-disks
virsh pool-undefine mulch-cloud-init
virsh pool-undefine mulch-backups
