#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9 / Ubuntu 18.10

. /etc/mulch.env

# add user
sudo useradd -m -s /bin/bash nosee || exit $?

# change home dir mode
sudo chmod 700 /home/nosee/ || exit $?

# copy SSH key (+ mode)
sudo mkdir /home/nosee/.ssh/ || exit $?
sudo cp /home/$_MULCH_SUPER_USER/.ssh/authorized_keys /home/nosee/.ssh/ || exit $?
sudo chown -R nosee:nosee /home/nosee/.ssh/ || exit $?
sudo chmod 700 /home/nosee/.ssh/ || exit $?
sudo chmod 600 /home/nosee/.ssh/authorized_keys || exit $?
