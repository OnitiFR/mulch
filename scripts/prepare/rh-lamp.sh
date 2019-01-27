#!/bin/bash

# -- Run with sudo privileges
# For: CentOS 7

appenv="/home/$_APP_USER/env"
html_dir="/home/$_APP_USER/public_html/"

# TODO:
# phpMyAdmin

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

sudo bash -c "cat > /etc/httpd/conf.d/000-default.conf" <<- EOS
User $_APP_USER
Group $_APP_USER

<Directory $html_dir>
    Options Indexes FollowSymLinks
    # Options is for .htaccess PHP settings and MultiViews
    # FileInfo is for rewrite
    # AuthConfig for Require
    # Indexes for expires
    AllowOverride Options=MultiViews FileInfo Limit AuthConfig Indexes
    Require all granted
</Directory>

<VirtualHost *:80>
    ServerAdmin webmaster@localhost
    DocumentRoot /home/$_APP_USER/public_html

    ErrorLog logs/error.log
    CustomLog logs/access.log combined
</VirtualHost>
EOS
[ $? -eq 0 ] || exit $?


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
grep ^export /etc/mulch.env | sed 's/^export //' >> $file
cat "/home/$_APP_USER/env" >> $file
EOS
[ $? -eq 0 ] || exit $?

sudo chmod +x $http_env  || exit $?

sudo touch $file # so systemd will not complain on 1st start
sudo sed -i "/\[Service\]/ a EnvironmentFile=$file" /usr/lib/systemd/system/httpd.service
sudo sed -i "/\[Service\]/ a ExecStartPre=$http_env" /usr/lib/systemd/system/httpd.service
sudo systemctl daemon-reload

echo "restart apache2"
sudo systemctl restart httpd || exit $?
