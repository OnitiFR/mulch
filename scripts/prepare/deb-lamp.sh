#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9 / Ubuntu 18.10

appenv="/home/$_APP_USER/env"
html_dir="/home/$_APP_USER/public_html/"

export DEBIAN_FRONTEND="noninteractive"
sudo -E apt-get -y -qq install apache2 php mariadb-server phpmyadmin pwgen || exit $?

MYSQL_PASSWORD=$(pwgen -1 16)

# Warning: see below for MariaDB user/db creation
sudo bash -c "cat > $appenv" <<- EOS
# local env for application
MYSQL_HOST="127.0.0.1"
MYSQL_DB="$_APP_USER"
MYSQL_USER="$_APP_USER"
MYSQL_PASSWORD="$MYSQL_PASSWORD"
HTML_DIR="$html_dir"
EOS
[ $? -eq 0 ] || exit $?

sudo bash -c "echo 'export \$(grep -v ^\# $appenv | xargs)' >> /home/$_APP_USER/.bashrc"

sudo mkdir -p $html_dir || exit $?
echo "creating/overwriting index.php..."
sudo bash -c "echo '<?php echo getenv(\"_VM_NAME\").\" is ready!\";' > $html_dir/index.php"

sudo chown -R $_APP_USER:$_APP_USER $html_dir $appenv || exit $?

# run Apache as $_APP_USER
sudo sed -i "s/APACHE_RUN_USER=www-data/APACHE_RUN_USER=$_APP_USER/" /etc/apache2/envvars || exit $?
sudo sed -i "s/APACHE_RUN_GROUP=www-data/APACHE_RUN_GROUP=$_APP_USER/" /etc/apache2/envvars || exit $?

sudo bash -c "cat > /etc/apache2/sites-available/000-default.conf" <<- EOS
<Directory $html_dir>
    Options Indexes FollowSymLinks
    # Options is for .htaccess PHP settings
    # FileInfo is for rewrite
    # AuthConfig for Require
    AllowOverride Options FileInfo Limit AuthConfig
    Require all granted
</Directory>

<VirtualHost *:80>
    ServerAdmin webmaster@localhost
    DocumentRoot /home/$_APP_USER/public_html

    ErrorLog \${APACHE_LOG_DIR}/error.log
    CustomLog \${APACHE_LOG_DIR}/access.log combined
</VirtualHost>
EOS
[ $? -eq 0 ] || exit $?

# change to /_mysql instead of /phpmyadmin
sudo sed -i 's|Alias /phpmyadmin|Alias /_sql|' /etc/phpmyadmin/apache.conf || exit $?

sudo bash -c "cat >> /etc/phpmyadmin/apache.conf" <<- EOS
<Directory /usr/share/phpmyadmin>
        php_admin_value upload_max_filesize 64M
        php_admin_value post_max_size 64M
</Directory>
EOS
[ $? -eq 0 ] || exit $?

sudo ln -s /etc/phpmyadmin/apache.conf /etc/apache2/conf-available/phpmyadmin.conf || exit $?
sudo a2enconf phpmyadmin || exit $?
sudo a2enmod rewrite || exit $?

# mysql_secure_installation
# In recent release of MariaDB, root access is only possible
# using "sudo mysql", so we don't have that much to do, here…
sudo bash -c "cat | mysql -sfu root" <<- EOS
DELETE FROM mysql.user WHERE User='';
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOS

# create mysql user and db (use args?)
sudo bash -c "cat | mysql -su root" <<- EOS
CREATE DATABASE IF NOT EXISTS $_APP_USER;
CREATE USER IF NOT EXISTS '$_APP_USER'@'localhost';
SET PASSWORD FOR '$_APP_USER'@'localhost' = PASSWORD('$MYSQL_PASSWORD');
GRANT ALL ON $_APP_USER.* TO '$_APP_USER'@'localhost';
FLUSH PRIVILEGES;
EOS
[ $? -eq 0 ] || exit $?

echo "restart apache2"
sudo bash -c "echo '. /etc/mulch.env' >> /etc/apache2/envvars"
sudo systemctl restart apache2 || exit $?
