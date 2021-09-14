#!/bin/bash

if [ -z "$1" ]; then
    echo "error: You must provide a username as argument"
    exit 1
fi

user_login="$1"

set -a
. ~/env
set +a

cd $HTML_DIR || exit $?


filename="$(pwgen 32).php" || exit $?
fullname="auth/$filename"
mkdir -p auth/ || exit $?

cat > $fullname <<- EOS
<?php
require_once("../wp-load.php");

\$user = get_userdatabylogin('${user_login}');
if (!\$user) {
    die('Unable to find the user ${user_login}.');
}

wp_set_auth_cookie(\$user->ID);
unlink(__FILE__);
header('Location: /wp-admin/');
EOS
[ $? -eq 0 ] || exit $?

echo "_MULCH_OPEN_URL=https://$_DOMAIN_FIRST/auth/$filename"
