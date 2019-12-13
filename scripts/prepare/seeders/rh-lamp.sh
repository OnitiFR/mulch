#!/bin/bash

# -- Run with sudo privileges
# For: CentOS 7

sudo yum -y install mod_php mariadb-server php-mysql php-mbstring php-intl php-xml php-gd || exit $?

if [ ! -d /usr/local/lib/phpMyAdmin ]; then
    # phpMyAdmin (old, unsupported, PHP 5.4 compliant version of phpMyAdmin)
    url="https://files.phpmyadmin.net/phpMyAdmin/4.0.10.20/phpMyAdmin-4.0.10.20-all-languages.tar.gz"
    sudo curl -s $url --output /usr/local/lib/pma.tgz || exit $?
    sudo tar xzf /usr/local/lib/pma.tgz -C /usr/local/lib || exit $?
    sudo rm -f /usr/local/lib/pma.tgz
    sudo rm -rf /usr/local/lib/phpMyAdmin/
    sudo mv /usr/local/lib/phpMyAdmin-* /usr/local/lib/phpMyAdmin || exit $?
fi
