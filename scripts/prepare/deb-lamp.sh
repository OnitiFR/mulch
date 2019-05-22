#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9 / Ubuntu 18.04 to 19.04

appenv="/home/$_APP_USER/env"
html_dir="/home/$_APP_USER/public_html/"

export DEBIAN_FRONTEND="noninteractive"
sudo -E apt-get -y -qq install apache2 php php-bcmath php-imagick mariadb-server phpmyadmin pwgen || exit $?

MYSQL_PASSWORD=$(pwgen -1 16)
[ $? -eq 0 ] || exit $?

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

sudo bash -c "echo 'export \$(grep -v ^\# $appenv | xargs)' >> /home/$_APP_USER/.bashrc" || exit $?

sudo mkdir -p $html_dir || exit $?
echo "creating/overwriting index.php..."
sudo bash -c "echo '<?php echo getenv(\"_VM_NAME\") . \" is ready!\";' > $html_dir/index.php" || exit $?

sudo chown -R $_APP_USER:$_APP_USER $html_dir $appenv || exit $?

# run Apache as $_APP_USER
sudo sed -i "s/APACHE_RUN_USER=www-data/APACHE_RUN_USER=$_APP_USER/" /etc/apache2/envvars || exit $?
sudo sed -i "s/APACHE_RUN_GROUP=www-data/APACHE_RUN_GROUP=$_APP_USER/" /etc/apache2/envvars || exit $?

sudo sed -i 's/^ServerTokens \(.\+\)$/ServerTokens Prod/' /etc/apache2/conf-enabled/security.conf || exit $?

sudo bash -c "cat > /etc/apache2/sites-available/000-default.conf" <<- EOS
# Allow mod_status even if we use RewriteEngine
<Location /server-status>
    RewriteEngine off
</Location>

<Directory $html_dir>
    Options Indexes FollowSymLinks
    # Options is for .htaccess PHP settings and MultiViews
    # FileInfo is for rewrite
    # AuthConfig for Require
    # Indexes for expires
    AllowOverride Options=MultiViews,Indexes FileInfo Limit AuthConfig Indexes
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

# change to /_sql instead of /phpmyadmin
sudo sed -i 's|Alias /phpmyadmin|Alias /_sql|' /etc/phpmyadmin/apache.conf || exit $?

sudo bash -c "cat >> /etc/phpmyadmin/apache.conf" <<- EOS
<Directory /usr/share/phpmyadmin>
        php_admin_value upload_max_filesize 64M
        php_admin_value post_max_size 64M
</Directory>
EOS
[ $? -eq 0 ] || exit $?

# bug: phpMyAdmin < 4.8 + PHP 7.2 = count() error
# tested targets: Ubuntu 18.10 / 19.04
# remaining tests: Ubuntu 16.04, Debian 9
if grep --quiet -E 'Ubuntu 18.10|Ubuntu 19.04' /etc/issue; then
    sudo sed -i "s/|\s*\((count(\$analyzed_sql_results\['select_expr'\]\)/| (\1)/g" /usr/share/phpmyadmin/libraries/sql.lib.php || exit $?
fi

sudo ln -s /etc/phpmyadmin/apache.conf /etc/apache2/conf-available/phpmyadmin.conf || exit $?
sudo a2enconf phpmyadmin || exit $?
sudo a2enmod rewrite expires || exit $?

sudo sed -i 's/^disable_functions = \(.*\)/disable_functions = \1phpinfo,/' /etc/php/*/apache2/php.ini

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

sudo bash -c "echo '. /etc/mulch.env' >> /etc/apache2/envvars" || exit $?
sudo bash -c "echo 'export \$(grep -v ^\# $appenv | xargs)' >> /etc/apache2/envvars" || exit $?

echo "restart apache2"
sudo systemctl restart apache2 || exit $?

echo "_MULCH_ACTION_NAME=db"
echo "_MULCH_ACTION_SCRIPT=https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/actions/deb_db_phpmyadmin.sh"
echo "_MULCH_ACTION_USER=admin"
echo "_MULCH_ACTION_DESCRIPTION=Login to phpMyAdmin"
echo "_MULCH_ACTION=commit"

echo "_MULCH_ACTION_NAME=logs"
echo "_MULCH_ACTION_SCRIPT=https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/actions/deb_apache_logs.sh"
echo "_MULCH_ACTION_USER=admin"
echo "_MULCH_ACTION_DESCRIPTION=Show live Apache logs"
echo "_MULCH_ACTION=commit"
