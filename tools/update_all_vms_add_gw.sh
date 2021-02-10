#!/bin/bash

# This script will add the GATEWAY nwfilter parameter to all existing VMs.
# - Was needed when when we switched from clean-traffic to clean-traffic-gateway
# - VMs must be rebooted to "apply" this new param

# Migration plan:
# 1 - copy new nwfilter.xml & vm.xml templates to your mulchd installation
# 2 - launch this script
# 3 - reboot all VMs (or reboot hypervisor if you're lazy)
# 4 - use "nwfilter-edit mulch-filter" and:
#       - switch to clean-traffic-gateway
#       - add DHCP request accept (see templates/nwfilter.xml)

gateway_mac=$(virsh net-dumpxml mulch | grep '<mac ' | cut -d\' -f 2)

hosts=$(virsh net-dumpxml mulch | grep '<host ')

while read -r host; do
    name=$(echo "$host" | cut -d\' -f 4)

    virsh dumpxml --inactive "$name" | grep "'GATEWAY_MAC'" > /dev/null
    if [ $? -eq 0 ]; then
        echo "already done for $name"
        continue
    fi

    EDITOR="sed -i \"/<parameter name='IP'/a <parameter name='GATEWAY_MAC' value='$gateway_mac'/>\"" virsh edit "$name"

done <<< "$hosts"
