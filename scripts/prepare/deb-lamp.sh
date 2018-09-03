#!/bin/bash

# Unlike RedHat/CentOS, Debian does not source profile for non-login shells:
. /etc/profile.d/vm.sh

export DEBIAN_FRONTEND="noninteractive"
sudo -E apt-get -y -qq install apache2 php mariadb-server phpmyadmin pwgen || exit $?

html_dir="/home/$_APP_USER/public_html/"

sudo mkdir -p $html_dir
echo "creating/overwriting index.php..."
sudo bash -c "echo '<?php echo \"App Server Ready!\";' > $html_dir/index.php"

sudo chown -R $_APP_USER:$_APP_USER $html_dir
sudo chmod 710 /home/$_APP_USER/
sudo chgrp www-data /home/$_APP_USER/

sudo bash -c "cat > /etc/apache2/sites-available/000-default.conf" <<- EOS
<Directory $html_dir>
    Options Indexes FollowSymLinks
    AllowOverride None
    Require all granted
</Directory>

<VirtualHost *:80>
    ServerAdmin webmaster@localhost
    DocumentRoot /home/$_APP_USER/public_html

    ErrorLog \${APACHE_LOG_DIR}/error.log
    CustomLog \${APACHE_LOG_DIR}/access.log combined
</VirtualHost>
EOS

sudo a2enmod rewrite || exit $?
sudo apachectl restart || exit $?

# mysql_secure_installation
# create mysql user and db (use args?)
# CREATE USER newuser@localhost IDENTIFIED BY 'password'; …
