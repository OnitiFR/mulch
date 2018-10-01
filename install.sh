#!/bin/bash

# run as root? if yes, we must also have non-root user
# if not, how do we create dirs?

# go install ./cmd... ?
# generate ssh key (if needed?)
# copy binaries?
# copy etc/ with templates (ex: /etc/mulch)
    # do not overwrite (in this case, warn the user)
# create var/data (ex: /var/lib/mulch)
# create var/storage (ex: /srv/mulch)
# create services?
# API key? (generate a new one?)
# check storage accessibility (minimum: --x) for libvirt
# check user privileges about libvirt (= is in libvirt group?)
# check if libvirt is running?
# → for last two checks: virsh -c qemu:///system

# - check that your user is in `libvirt` group
#    - some distributions do this automatically on package install
#    - you may have to disconnect / reconnect your user
#    - if needed: `usermod -aG libvirt $USER`
