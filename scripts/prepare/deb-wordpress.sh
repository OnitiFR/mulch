#!/bin/bash

# -- Run as user. WILL DELETE public_html CONTENT

# Unlike RedHat/CentOS, Debian does not source profile for non-login shells:
. /etc/mulch.env
. ~/env

# From Wordpress Dockerfile
WORDPRESS_VERSION="4.9.8"
WORDPRESS_SHA1="0945bab959cba127531dceb2c4fed81770812b4f"

mkdir -p tmp || exit $?
echo "downloading Wordpress $WORDPRESS_VERSION"

curl -so wordpress.tar.gz -fSL "https://wordpress.org/wordpress-${WORDPRESS_VERSION}.tar.gz" || exit $?
echo "$WORDPRESS_SHA1 *wordpress.tar.gz" | sha1sum -c - || exit $?

echo "extracting Wordpress"
# upstream tarballs include ./wordpress/
tar -xzf wordpress.tar.gz -C tmp || exit $?
rm wordpress.tar.gz || exit $?
rm -rf $HTML_DIR # !!!
mkdir $HTML_DIR || exit $?
mv -f tmp/wordpress/* $HTML_DIR || exit $?

cd $HTML_DIR || exit $?

cat > .htaccess <<- EOS
# BEGIN WordPress
<IfModule mod_rewrite.c>
RewriteEngine On
RewriteBase /
RewriteRule ^index\.php$ - [L]
RewriteCond %{REQUEST_FILENAME} !-f
RewriteCond %{REQUEST_FILENAME} !-d
RewriteRule . /index.php [L]
</IfModule>
# END WordPress
EOS

# version 4.4.1 decided to switch to windows line endings
sed -ri -e 's/\r$//' wp-config*


# used as a test for next step ("awk HTTPS thingy")
cp -f wp-config-sample.php wp-config.php || exit $?

awk '/^\/\*.*stop editing.*\*\/$/ && c == 0 { c = 1; system("cat") } { print }' wp-config-sample.php > wp-config.php <<'EOPHP'
// If we're behind a proxy server and using HTTPS, we need to alert Wordpress of that fact
// see also http://codex.wordpress.org/Administration_Over_SSL#Using_a_Reverse_Proxy
if (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {
    $_SERVER['HTTPS'] = 'on';
}

EOPHP

sed_escape_lhs() {
    echo "$@" | sed -e 's/[]\/$*.^|[]/\\&/g'
}
sed_escape_rhs() {
    echo "$@" | sed -e 's/[\/&]/\\&/g'
}
php_escape() {
    local escaped="$(php -r 'var_export(('"$2"') $argv[1]);' -- "$1")"
    if [ "$2" = 'string' ] && [ "${escaped:0:1}" = "'" ]; then
        escaped="${escaped//$'\n'/"' + \"\\n\" + '"}"
    fi
    echo "$escaped"
}
set_config() {
    key="$1"
    value="$2"
    var_type="${3:-string}"
    start="(['\"])$(sed_escape_lhs "$key")\2\s*,"
    end="\);"
    if [ "${key:0:1}" = '$' ]; then
        start="^(\s*)$(sed_escape_lhs "$key")\s*="
        end=";"
    fi
    sed -ri -e "s/($start\s*).*($end)$/\1$(sed_escape_rhs "$(php_escape "$value" "$var_type")")\3/" wp-config.php
}

set_config 'DB_HOST' "$MYSQL_HOST"
set_config 'DB_USER' "$MYSQL_USER"
set_config 'DB_PASSWORD' "$MYSQL_PASSWORD"
set_config 'DB_NAME' "$MYSQL_DB"
# set_config 'DB_CHARSET' "$WORDPRESS_DB_CHARSET"
# set_config 'DB_COLLATE' "$WORDPRESS_DB_COLLATE"

# Note: Added at the end of wp-config.php, it seems.
# // If we're behind a proxy server and using HTTPS, we need to alert Wordpress of that fact
# // see also http://codex.wordpress.org/Administration_Over_SSL#Using_a_Reverse_Proxy
# if (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {
# 	$_SERVER['HTTPS'] = 'on';
# }
