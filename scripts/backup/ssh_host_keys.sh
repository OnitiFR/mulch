#!/bin/bash

# -- Run as admin user

cd /etc/ssh/ || exit $?

sudo tar cf $_BACKUP/ssh-host-keys.tar *key* || exit $?
