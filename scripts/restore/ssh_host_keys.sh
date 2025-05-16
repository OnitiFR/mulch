#!/bin/bash

# -- Run as admin user

cd /etc/ssh/ || exit $?

sudo tar xf $_BACKUP/ssh-host-keys.tar || exit $?

ssh_unit=$(systemctl list-unit-files | cut -d\  -f1 | grep 'sshd*\.service' | head -n1)

echo "Restarting $ssh_unit for new host keys"
sudo systemctl restart $ssh_unit || exit $?

