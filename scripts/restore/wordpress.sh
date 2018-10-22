#!/bin/bash

. /etc/mulch.env
. ~/env

rm -rf "$HTML_DIR" || exit $?
mkdir "$html_dir" || exit $?


cd "$HTML_DIR" || exit $?

tar xf "$_BACKUP/app.tar" || exit $?

# version 4.4.1 decided to switch to windows line endings
sed -ri -e 's/\r$//' wp-config*

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

# password on the command line? brrr…
mysql -u $MYSQL_USER -h $MYSQL_HOST "-p$MYSQL_PASSWORD" $MYSQL_DB < "$_BACKUP/app.sql" || exit $?

# Should also set SITEURL and HOME (in option table? in wp-config?)
# (using URL provided by: lamp script? mulch itself?)
