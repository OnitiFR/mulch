#!/bin/bash

# -- Run with sudo privileges
# For: CentOS 7

# You may define MULCH_HTTP_BASIC_AUTH env var for a simple basic
# HTTP authentication (format user:password, avoid special shell chars)

appenv="/home/$_APP_USER/env"
html_dir="/home/$_APP_USER/public_html/"

sudo yum -y install mod_php mariadb-server php-mysql php-mbstring php-intl php-xml php-gd || exit $?

sudo systemctl enable -q httpd || exit $?
sudo systemctl enable -q --now mariadb || exit $?

# currently disabling selinux (hard to use the right contexts allowing
# a generic use of $html_dir without opening everything)
sudo bash -c "cat > /etc/selinux/config" <<- EOS
SELINUX=disabled
SELINUXTYPE=targeted
EOS
[ $? -eq 0 ] || exit $?

sudo setenforce 0 || exit $?

genpasswd() {
    strings /dev/urandom | grep -o '[[:alnum:]]' | head -n 14 | tr -d '\n'; echo
}

MYSQL_PASSWORD=$(genpasswd)
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
sudo bash -c "echo '<?php echo getenv(\"_VM_NAME\") . \" is ready! - see \" . getenv(\"HTML_DIR\");' > $html_dir/index.php" || exit $?

sudo chown -R $_APP_USER:$_APP_USER $html_dir $appenv || exit $?

# empty a few config files
sudo bash -c "echo > /etc/httpd/conf.d/welcome.conf" || exit $?
sudo bash -c "echo > /etc/httpd/conf.d/userdir.conf" || exit $?

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

sudo bash -c "cat > /etc/httpd/conf.d/000-default.conf" <<- EOS
User $_APP_USER
Group $_APP_USER
ServerTokens Prod

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

    # combined with real IP + response time in ms (2.4.13 needed, CentOS 7 will fall back to seconds)
    LogFormat "%{X-Real-Ip}i %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-Agent}i\" %{ms}T" combined_real_plus
    ErrorLog logs/error_log
    CustomLog logs/access_log combined_real_plus

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

sudo chgrp $_APP_USER /var/lib/php/session/ || exit $?

sudo sed -i 's/^disable_functions \(.\+\)$/disable_functions = phpinfo/' /etc/php.ini || exit $?
sudo sed -i 's/^expose_php\(.\+\)$/expose_php = Off/' /etc/php.ini || exit $?

if [ ! -f ~/.my.cnf ]; then
  root_pwd=$(genpasswd)

  # mysql_secure_installation
  mysql -sfu root <<- EOS
UPDATE mysql.user set Password=PASSWORD('$root_pwd') WHERE User='root';
DELETE FROM mysql.user WHERE User='';
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOS

  cat > ~/.my.cnf <<- EOS
[client]
user=root
password=$root_pwd
EOS
  [ $? -eq 0 ] || exit $?
fi

# create mysql user and db (use args?)
mysql -su root <<- EOS
CREATE DATABASE IF NOT EXISTS $_APP_USER;
GRANT ALL ON $_APP_USER.* TO '$_APP_USER'@'localhost';
SET PASSWORD FOR '$_APP_USER'@'localhost' = PASSWORD('$MYSQL_PASSWORD');
FLUSH PRIVILEGES;
EOS
[ $? -eq 0 ] || exit $?

# inject env into Apache
http_env="/usr/local/bin/httpd_env"
file="/home/$_MULCH_SUPER_USER/.httpd_env"

sudo bash -c "cat > $http_env" <<- EOS
#!/bin/bash
echo "# generated, do not modify" > $file
grep ^export /etc/mulch.env | sed 's/; /\n/g' | sed 's/^export //' >> $file
cat "/home/$_APP_USER/env" >> $file
EOS
[ $? -eq 0 ] || exit $?

sudo chmod +x $http_env  || exit $?

sudo touch $file # so systemd will not complain on 1st start
sudo sed -i "/\[Service\]/ a EnvironmentFile=$file" /usr/lib/systemd/system/httpd.service || exit $?
sudo sed -i "/\[Service\]/ a ExecStartPre=$http_env" /usr/lib/systemd/system/httpd.service || exit $?
sudo systemctl daemon-reload || exit $?

# phpMyAdmin (old, unsupported, PHP 5.4 compliant version of phpMyAdmin)
url="https://files.phpmyadmin.net/phpMyAdmin/4.0.10.20/phpMyAdmin-4.0.10.20-all-languages.tar.gz"
sudo curl -s $url --output /usr/local/lib/pma.tgz || exit $?
sudo tar xzf /usr/local/lib/pma.tgz -C /usr/local/lib || exit $?
sudo rm -f /usr/local/lib/pma.tgz
sudo rm -rf /usr/local/lib/phpMyAdmin/
sudo mv /usr/local/lib/phpMyAdmin-* /usr/local/lib/phpMyAdmin || exit $?

sudo bash -c "cat > /usr/local/lib/phpMyAdmin/config.inc.php" <<- 'EOS'
<?php
# fix for :80 with this (old) phpMyAdmin release
$cfg['PmaAbsoluteUri'] = "https://" . getenv('_DOMAIN_FIRST') . "/_sql/";
$cfg['LoginCookieValidity'] = 86400;
ini_set('session.gc_maxlifetime', '86400');
EOS
[ $? -eq 0 ] || exit $?

sudo bash -c "cat > /etc/httpd/conf.d/001-phpmyadmin.conf" <<- EOS
# phpMyAdmin default Apache configuration

Alias /_sql /usr/local/lib/phpMyAdmin

<Directory /usr/local/lib/phpMyAdmin>
    Require all granted
    php_admin_value mbstring.func_overload 0
</Directory>

# Disallow web access to directories that don't need it
<Directory /usr/local/lib/phpMyAdmin/templates>
    Require all denied
</Directory>
<Directory /usr/local/lib/phpMyAdmin/libraries>
    Require all denied
</Directory>
<Directory /usr/local/lib/phpMyAdmin/setup/lib>
    Require all denied
</Directory>

<Directory /usr/local/lib/phpMyAdmin>
        php_admin_value upload_max_filesize 64M
        php_admin_value post_max_size 64M
</Directory>
EOS
[ $? -eq 0 ] || exit $?

echo "restart apache2"
sudo systemctl restart httpd || exit $?

echo "_MULCH_ACTION_NAME=db"
echo "_MULCH_ACTION_SCRIPT={core}/actions/rh_db_phpmyadmin.sh"
echo "_MULCH_ACTION_USER=$_MULCH_SUPER_USER"
echo "_MULCH_ACTION_DESCRIPTION=Login to phpMyAdmin"
echo "_MULCH_ACTION=commit"

echo "_MULCH_ACTION_NAME=logs"
echo "_MULCH_ACTION_SCRIPT={core}/actions/rh_apache_logs.sh"
echo "_MULCH_ACTION_USER=$_MULCH_SUPER_USER"
echo "_MULCH_ACTION_DESCRIPTION=Show live Apache logs"
echo "_MULCH_ACTION=commit"
