#!/bin/bash

# -- Run with sudo privileges
# For: Debian / Ubuntu

export DEBIAN_FRONTEND="noninteractive"
sudo -E apt-get -y -qq install apache2 php php-intl php-bcmath php-imagick pwgen postgresql postgresql-client php-pgsql || exit $?
