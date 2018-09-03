#!/bin/bash

export DEBIAN_FRONTEND="noninteractive"
sudo -E apt-get -y -qq install apache2 php mariadb-server phpmyadmin pwgen || exit $?

# change document root to app (get this name from ENV? args?)
# chown -R www-data:www-data OR change apache2 user?
# create template PHP website
# mysql_secure_installation
# create mysql user and db (use args?)
# CREATE USER newuser@localhost IDENTIFIED BY 'password'; …
# a2enmod rewrite
# reload d'apache?
