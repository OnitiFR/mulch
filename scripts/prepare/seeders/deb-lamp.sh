#!/bin/bash

# -- Run with sudo privileges
# For: Debian / Ubuntu

export DEBIAN_FRONTEND="noninteractive"
# NB: second line (mysql, curl, …) install phpMyAdmin dependencies
sudo -E apt-get -y -qq install apache2 php \
    php-mysql php-curl php-zip php-bz2 php-gd php-mbstring php-xml php-pear \
    php-intl php-bcmath php-imagick \
    mariadb-server pwgen || exit $?

# no more available with Ubuntu 20.04, packaged phpMyAdmin will use motranslator/shapefile instead
sudo -E apt-get -y -qq install php-php-gettext 2> /dev/null


sudo -E apt-get -y -qq install phpmyadmin 2> /dev/null
sudo -E apt-get -y -qq install jq 2> /dev/null
