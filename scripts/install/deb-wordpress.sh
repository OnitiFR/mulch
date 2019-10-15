#!/bin/bash

# -- Run as app user. WILL DELETE public_html CONTENT
# Inspired from Wordpress Dockerfile

. ~/env

# https://wordpress.org/download/releases/ (tar.gz)
WORDPRESS_VERSION="5.2.4"
WORDPRESS_SHA1="9eb002761fc8b424727d8c9d291a6ecfde0c53b7"

mkdir -p tmp || exit $?
echo "downloading Wordpress $WORDPRESS_VERSION ($WORDPRESS_SHA1)"

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
php_value upload_max_filesize 64M
php_value post_max_size 64M

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
[ $? -eq 0 ] || exit $?

echo "configuring Wordpress"

# version 4.4.1 decided to switch to windows line endings
sed -ri -e 's/\r$//' wp-config* || exit $?

# used as a test for next step ("awk HTTPS thingy")
cp -f wp-config-sample.php wp-config.php || exit $?

awk '/^\/\*.*stop editing.*\*\/$/ && c == 0 { c = 1; system("cat") } { print }' wp-config-sample.php > wp-config.php <<'EOPHP'
// If we're behind a proxy server and using HTTPS, we need to alert Wordpress of that fact
// see also http://codex.wordpress.org/Administration_Over_SSL#Using_a_Reverse_Proxy
if (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {
    $_SERVER['HTTPS'] = 'on';
}

EOPHP
[ $? -eq 0 ] || exit $?

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
# no php_escape in this variant
set_config_raw() {
    key="$1"
    value="$2"
    var_type="${3:-string}"
    start="(['\"])$(sed_escape_lhs "$key")\2\s*,"
    end="\);"
    if [ "${key:0:1}" = '$' ]; then
        start="^(\s*)$(sed_escape_lhs "$key")\s*="
        end=";"
    fi
    sed -ri -e "s/($start\s*).*($end)$/\1$(sed_escape_rhs "$value")\3/" wp-config.php || exit $?
}

set_config_raw 'DB_HOST' "getenv('MYSQL_HOST')"
set_config_raw 'DB_USER' "getenv('MYSQL_USER')"
set_config_raw 'DB_PASSWORD' "getenv('MYSQL_PASSWORD')"
set_config_raw 'DB_NAME' "getenv('MYSQL_DB')"
# set_config 'DB_CHARSET' "$WORDPRESS_DB_CHARSET"
# set_config 'DB_COLLATE' "$WORDPRESS_DB_COLLATE"

uniqueEnvs=(
    AUTH_KEY
    SECURE_AUTH_KEY
    LOGGED_IN_KEY
    NONCE_KEY
    AUTH_SALT
    SECURE_AUTH_SALT
    LOGGED_IN_SALT
    NONCE_SALT
)

for unique in "${uniqueEnvs[@]}"; do
	uniqVar="WORDPRESS_$unique"
	if [ -n "${!uniqVar}" ]; then
		set_config "$unique" "${!uniqVar}"
	else
		# if not specified, let's generate a random value
		currentVal="$(sed -rn -e "s/define\(\s*(([\'\"])$unique\2\s*,\s*)(['\"])(.*)\3\s*\);/\4/p" wp-config.php)"
		if [ "$currentVal" = 'put your unique phrase here' ]; then
			set_config "$unique" "$(head -c1m /dev/urandom | sha1sum | cut -d' ' -f1)"
		fi
	fi
done
