#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9+ / Ubuntu 18.10+

# You may define MULCH_HTTP_BASIC_AUTH env var for a simple basic
# HTTP authentication (format user:password, avoid special shell chars)

appenv="/home/$_APP_USER/env"
html_dir="/home/$_APP_USER/public_html/"
pgpass="/home/$_APP_USER/.pgpass"

export DEBIAN_FRONTEND="noninteractive"

sudo -E apt-get -y -qq install apache2 php php-intl php-bcmath php-imagick pwgen postgresql postgresql-client php-pgsql || exit $?

PGSQL_PASSWORD=$(pwgen -1 16)
[ $? -eq 0 ] || exit $?

sudo bash -c "cat > $appenv" <<- EOS
# local env for application
PGSQL_HOST="127.0.0.1"
PGSQL_DB="$_APP_USER"
PGSQL_USER="$_APP_USER"
PGSQL_PASSWORD="$PGSQL_PASSWORD"
HTML_DIR="$html_dir"
EOS
[ $? -eq 0 ] || exit $?

sudo bash -c "echo 'export \$(grep -v ^\# $appenv | xargs)' >> /home/$_APP_USER/.bashrc" || exit $?

sudo mkdir -p $html_dir || exit $?
echo "creating/overwriting index.php..."
sudo bash -c "echo '<?php echo getenv(\"_VM_NAME\").\" is ready!\";' > $html_dir/index.php" || exit $?

sudo bash -c "echo 'localhost:5432:$_APP_USER:$_APP_USER:$PGSQL_PASSWORD' > $pgpass" || exit $?

sudo chown -R $_APP_USER:$_APP_USER $html_dir $appenv $pgpass || exit $?
sudo chmod 600 $pgpass || exit $?

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
    Require valid-user"
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

    # compression
    AddOutputFilterByType DEFLATE text/css

    AddOutputFilterByType DEFLATE text/plain
    AddOutputFilterByType DEFLATE text/html
    AddOutputFilterByType DEFLATE text/xml
    AddOutputFilterByType DEFLATE application/xml
    AddOutputFilterByType DEFLATE application/xhtml+xml
    AddOutputFilterByType DEFLATE application/javascript
    AddOutputFilterByType DEFLATE application/x-javascript
    AddOutputFilterByType DEFLATE text/javascript
    AddOutputFilterByType DEFLATE application/json

    AddOutputFilterByType DEFLATE application/x-font
    AddOutputFilterByType DEFLATE application/x-font-opentype
    AddOutputFilterByType DEFLATE application/x-font-otf
    AddOutputFilterByType DEFLATE application/x-font-truetype
    AddOutputFilterByType DEFLATE application/x-font-ttf
    AddOutputFilterByType DEFLATE application/vnd.ms-fontobject
    AddOutputFilterByType DEFLATE font/opentype
    AddOutputFilterByType DEFLATE font/otf
    AddOutputFilterByType DEFLATE font/ttf

    AddOutputFilterByType DEFLATE image/svg+xml
    AddOutputFilterByType DEFLATE image/x-icon
</VirtualHost>
EOS
[ $? -eq 0 ] || exit $?

sudo a2enmod rewrite expires || exit $?

sudo sed -i 's/^disable_functions = \(.*\)/disable_functions = \1phpinfo,/' /etc/php/*/apache2/php.ini || exit $?

# Adminer
adminer_url="https://www.adminer.org/latest.php"
sudo mkdir -p /usr/share/adminer || exit $?
sudo wget -q -O /usr/share/adminer/adminer.php "$adminer_url" || exit $?

sudo bash -c "cat > /etc/apache2/conf-available/adminer.conf" <<- EOS
Alias /_sql /usr/share/adminer

<Directory /usr/share/adminer>
    Options SymLinksIfOwnerMatch
    DirectoryIndex index.php

    php_admin_value upload_max_filesize 64M
    php_admin_value post_max_size 64M
</Directory>
EOS
[ $? -eq 0 ] || exit $?

sudo a2enconf adminer || exit $?

sudo bash -c "cat > /usr/share/adminer/index.php" <<- EOS
<?php
header('Location: adminer.php?pgsql=&username=${_APP_USER}&db=${_APP_USER}');
EOS
[ $? -eq 0 ] || exit $?

# PgSQL
sudo bash -c "cat > /etc/postgresql/*/main/pg_hba.conf" <<- EOS
# Modified by Mulch to remove "peer" auth for users (other than postgres)
# Database administrative login by Unix domain socket
local   all             postgres                                peer

# TYPE  DATABASE        USER            ADDRESS                 METHOD

# "local" is for Unix domain socket connections only
local   all             all                                     md5
# IPv4 local connections:
host    all             all             127.0.0.1/32            md5
# IPv6 local connections:
host    all             all             ::1/128                 md5
# Allow replication connections from localhost, by a user with the
# replication privilege.
local   replication     all                                     md5
host    replication     all             127.0.0.1/32            md5
host    replication     all             ::1/128                 md5
EOS
[ $? -eq 0 ] || exit $?

sudo systemctl restart postgresql || exit $?

sudo bash -c "cat | sudo -iu postgres psql -v ON_ERROR_STOP=1" <<- EOS
CREATE DATABASE $_APP_USER;
CREATE USER $_APP_USER WITH PASSWORD '$PGSQL_PASSWORD';
GRANT ALL PRIVILEGES ON DATABASE $_APP_USER to $_APP_USER;
\connect $_APP_USER
CREATE EXTENSION fuzzystrmatch;
EOS
[ $? -eq 0 ] || exit $?


echo "restart apache2"
sudo bash -c "echo '. /etc/mulch.env' >> /etc/apache2/envvars" || exit $?
sudo bash -c "echo 'export \$(grep -v ^\# $appenv | xargs)' >> /etc/apache2/envvars" || exit $?

sudo systemctl restart apache2 || exit $?

echo "_MULCH_ACTION_NAME=db"
echo "_MULCH_ACTION_SCRIPT=https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/actions/deb_db_adminer.sh"
echo "_MULCH_ACTION_USER=$_MULCH_SUPER_USER"
echo "_MULCH_ACTION_DESCRIPTION=Login to Adminer"
echo "_MULCH_ACTION=commit"

echo "_MULCH_ACTION_NAME=logs"
echo "_MULCH_ACTION_SCRIPT=https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/actions/deb_apache_logs.sh"
echo "_MULCH_ACTION_USER=$_MULCH_SUPER_USER"
echo "_MULCH_ACTION_DESCRIPTION=Show live Apache logs"
echo "_MULCH_ACTION=commit"
