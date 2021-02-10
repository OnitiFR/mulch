#!/bin/bash

# Will switch from hardcoded "clean-traffic" nwfiler to "mulch-filter".
# This script is now useless but is an interesting reference for future
# mass updates, if needed.

tofix=$(grep -iL "<filterref filter='clean-traffic'/>" /etc/libvirt/qemu/*.xml)
if [ -n "$tofix" ]; then
    echo "Please, set/fix/clean clean-traffic for:"
    echo "$tofix"
    exit 1
fi

hosts=$(virsh net-dumpxml mulch | grep '<host ')

while read -r host; do
    #echo "-$host-"

    name=$(echo "$host" | cut -d\' -f 4)
    ip=$(echo "$host" | cut -d\' -f 6)
    file="/etc/libvirt/qemu/${name}.xml"

    #echo "-$name/$ip-"
    #ls -l "$file"
    # <filterref filter='clean-traffic'/>

    sed -i "s|<filterref filter='clean-traffic'/>|<filterref filter='mulch-filter'><parameter name='IP' value='$ip'/></filterref>|" "$file"
done <<< "$hosts"
