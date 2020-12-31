#!/bin/bash

set -e
set -o pipefail

latest=$(curl -s https://cdimage.debian.org/cdimage/openstack/current-10/ | \
    sed -n 's/.*\(debian-10.[0-9.-]\+-openstack-amd64.qcow2\).*/\1/p' | \
    head -n1)

echo "https://cdimage.debian.org/cdimage/openstack/current-10/$latest"
