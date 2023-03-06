#!/bin/bash

if [ -n "$1" ]; then
    user_login="$1"
else
    user_login="$_CALLING_KEY"
fi

if [ -z "$user_login" ]; then
    >&2 echo "you must provide a username as argument"
    exit 1
fi

echo "note: you can provide a username as argument (default is '$_CALLING_KEY')"

set -a
. ~/env
set +a

cd $HTML_DIR || exit $?


filename="$(pwgen 32).php" || exit $?
fullname="auth/$filename"
mkdir -p auth/ || exit $?

cat > $fullname <<- EOS
<?php
require_once("../wp-config.php");
require_once(ABSPATH . "/wp-load.php");

\$user = get_userdatabylogin('${user_login}');
if (!\$user) {
    die('Unable to find the user ${user_login}.');
}

wp_set_auth_cookie(\$user->ID);
unlink(__FILE__);
header('Location: ' . parse_url(get_admin_url(), PHP_URL_PATH));
EOS
[ $? -eq 0 ] || exit $?

echo "_MULCH_OPEN_URL=https://$_DOMAIN_FIRST/auth/$filename"
