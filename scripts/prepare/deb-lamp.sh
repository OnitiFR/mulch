#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9 / Ubuntu 18.04 to 19.04

# You may define MULCH_HTTP_BASIC_AUTH env var for a simple basic
# HTTP authentication (format user:password, avoid special shell chars)

appenv="/home/$_APP_USER/env"
html_dir="/home/$_APP_USER/public_html/"

export DEBIAN_FRONTEND="noninteractive"
# NB: second line (mysql, curl, …) install phpMyAdmin dependencies
sudo -E apt-get -y -qq install apache2 php \
    php-mysql php-curl php-zip php-bz2 php-gd php-mbstring php-xml php-pear php-php-gettext \
    php-intl php-bcmath php-imagick \
    mariadb-server pwgen || exit $?

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

if [ -n "$MULCH_HTTP_BASIC_AUTH" ]; then
    # create htpasswd file
    htpasswd="/home/$_APP_USER/.htpasswd"
    IFS=':' read -r user password <<< "$MULCH_HTTP_BASIC_AUTH"
    echo "$password" | sudo htpasswd -ci $htpasswd "$user" || exit $?

    auth="AuthType Basic
    AuthName Authentication
    AuthUserFile $htpasswd
    Require valid-user
    "
else
    auth="Require all granted"
fi

sudo bash -c "cat > /etc/apache2/sites-available/000-default.conf" <<- EOS
# Allow mod_status even if we use RewriteEngine
<Location /server-status>
    <IfModule mod_rewrite.c>
        RewriteEngine off
    </IfModule>
</Location>

<Directory $html_dir>
    Options Indexes FollowSymLinks
    # Options is for .htaccess PHP settings and MultiViews
    # FileInfo is for rewrite
    # AuthConfig for Require
    # Indexes for expires
    AllowOverride Options=MultiViews,Indexes FileInfo Limit AuthConfig Indexes
    $auth
</Directory>

<VirtualHost *:80>
    ServerAdmin webmaster@localhost
    DocumentRoot $html_dir

    LogFormat "%{X-Real-Ip}i %l %u %t \"%r\" %>s %O \"%{Referer}i\" \"%{User-Agent}i\"" combined_real
    ErrorLog \${APACHE_LOG_DIR}/error.log
    CustomLog \${APACHE_LOG_DIR}/access.log combined_real
</VirtualHost>
EOS
[ $? -eq 0 ] || exit $?

sudo -E apt-get -y -qq install phpmyadmin
if [ $? -ne 0 ]; then
    # on Debian 10, phpMyAdmin is not available (yet?)
    sudo -E apt-get -y -qq install jq || exit $?
    latest="$(curl -fsSL 'https://www.phpmyadmin.net/home_page/version.json' | jq -r '.version')"

    url="https://files.phpmyadmin.net/phpMyAdmin/${latest}/phpMyAdmin-${latest}-all-languages.tar.gz"
    sudo curl -s $url --output /usr/local/lib/pma.tgz || exit $?
    sudo tar xzf /usr/local/lib/pma.tgz -C /usr/share || exit $?
    sudo rm -f /usr/local/lib/pma.tgz
    sudo rm -rf /usr/share/phpmyadmin/
    sudo mv /usr/share/phpMyAdmin-* /usr/share/phpmyadmin || exit $?
    # rm changelog, readme, etc…
    sudo rm -f /usr/share/phpmyadmin/[ABCDEFGHIJKLMNOPQRSTUVW]*

    sudo mkdir /usr/share/phpmyadmin/tmp || exit $?
    sudo chown $_APP_USER:$_APP_USER /usr/share/phpmyadmin/tmp || exit $?

    sudo bash -c "echo 'Auth failure' > /usr/share/phpmyadmin/auth.html" || exit $?

    sudo bash -c "cat > /usr/share/phpmyadmin/config.inc.php" <<- 'EOS'
<?php
$cfg['Servers'][1]['auth_type'] = 'signon';
$cfg['Servers'][1]['SignonSession'] = 'SignonSession';
$cfg['Servers'][1]['SignonURL'] = '/_sql/auth.html';
EOS
    [ $? -eq 0 ] || exit $?

    sudo bash -c "cat > /etc/apache2/conf-available/phpmyadmin.conf" <<- EOS
# phpMyAdmin default Apache configuration

Alias /_sql /usr/share/phpmyadmin

<Directory /usr/share/phpmyadmin>
    Require all granted
    php_admin_value mbstring.func_overload 0
</Directory>

# Disallow web access to directories that don't need it
<Directory /usr/share/phpmyadmin/templates>
    Require all denied
</Directory>
<Directory /usr/share/phpmyadmin/libraries>
    Require all denied
</Directory>
<Directory /usr/share/phpmyadmin/setup/lib>
    Require all denied
</Directory>

<Directory /usr/share/phpmyadmin>
        php_admin_value upload_max_filesize 64M
        php_admin_value post_max_size 64M
</Directory>
EOS
    [ $? -eq 0 ] || exit $?
else
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
    # tested targets: Ubuntu 18.04 / 18.10 / 19.04
    # remaining tests: Ubuntu 16.04, Debian 9
    if grep --quiet -E 'Ubuntu 18.04|Ubuntu 18.10|Ubuntu 19.04' /etc/issue; then
        sudo sed -i "s/|\s*\((count(\$analyzed_sql_results\['select_expr'\]\)/| (\1)/g" /usr/share/phpmyadmin/libraries/sql.lib.php || exit $?
    fi

    sudo ln -s /etc/phpmyadmin/apache.conf /etc/apache2/conf-available/phpmyadmin.conf || exit $?
fi

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
echo "_MULCH_ACTION_USER=$_MULCH_SUPER_USER"
echo "_MULCH_ACTION_DESCRIPTION=Login to phpMyAdmin"
echo "_MULCH_ACTION=commit"

echo "_MULCH_ACTION_NAME=logs"
echo "_MULCH_ACTION_SCRIPT=https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/actions/deb_apache_logs.sh"
echo "_MULCH_ACTION_USER=$_MULCH_SUPER_USER"
echo "_MULCH_ACTION_DESCRIPTION=Show live Apache logs"
echo "_MULCH_ACTION=commit"
