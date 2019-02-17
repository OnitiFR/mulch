#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9 / Ubuntu 18.10

appenv="/home/$_APP_USER/env"
html_dir="/home/$_APP_USER/public_html/"

export DEBIAN_FRONTEND="noninteractive"
sudo -E apt-get -y -qq install apache2 php pwgen postgresql postgresql-client php-pgsql || exit $?

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

sudo chown -R $_APP_USER:$_APP_USER $html_dir $appenv || exit $?

# run Apache as $_APP_USER
sudo sed -i "s/APACHE_RUN_USER=www-data/APACHE_RUN_USER=$_APP_USER/" /etc/apache2/envvars || exit $?
sudo sed -i "s/APACHE_RUN_GROUP=www-data/APACHE_RUN_GROUP=$_APP_USER/" /etc/apache2/envvars || exit $?

sudo sed -i 's/^ServerTokens \(.\+\)$/ServerTokens Prod/' /etc/apache2/conf-enabled/security.conf || exit $?

sudo bash -c "cat > /etc/apache2/sites-available/000-default.conf" <<- EOS
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

sudo a2enmod rewrite || exit $?
sudo a2enmod rewrite expires || exit $?

# install phpPgAdmin? (package: phppgadmin)

sudo bash -c "cat | sudo -iu postgres psql -v ON_ERROR_STOP=1" <<- EOS
CREATE DATABASE $_APP_USER;
CREATE USER $_APP_USER WITH PASSWORD '$PGSQL_PASSWORD';
GRANT ALL PRIVILEGES ON DATABASE $_APP_USER to $_APP_USER;
EOS
[ $? -eq 0 ] || exit $?


echo "restart apache2"
sudo bash -c "echo '. /etc/mulch.env' >> /etc/apache2/envvars" || exit $?
sudo bash -c "echo 'export \$(grep -v ^\# $appenv | xargs)' >> /etc/apache2/envvars" || exit $?

sudo systemctl restart apache2 || exit $?
