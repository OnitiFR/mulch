#!/bin/bash

# -- Run as admin user

cd /etc/ssh/ || exit $?

sudo tar xf $_BACKUP/ssh-host-keys.tar || exit $?

sudo systemctl restart sshd || exit $?

