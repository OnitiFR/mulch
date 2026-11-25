#!/bin/bash

user_login="$_CALLING_KEY"
echo_prefix='_MULCH_OPEN_URL='
validity_min=60

while [ $# -gt 0 ]; do
    case "$1" in
        --help)
            echo "Usage: $0 [username] [--url]"
            echo "  --url: print the URL instead of opening it"
            exit 0
            ;;
        --url)
            echo_prefix=''
            ;;
        *)
            user_login="$1"
            ;;
    esac
    shift
done

if [ -z "$user_login" ]; then
    >&2 echo "you must provide a username as argument"
    exit 1
fi

echo "note: you can provide a username as argument (default is '$_CALLING_KEY')"

set -a
. ~/env
set +a

cd "$HTML_DIR" || exit $?

# Resolve the user
user_id=$(wp user get "$user_login" --field=ID 2>/dev/null)
if ! [[ "$user_id" =~ ^[0-9]+$ ]]; then
    >&2 echo "unable to find user '$user_login', administrators are:"
    wp user list --role=administrator --field=user_login >&2
    exit 1
fi

token=$(pwgen -s 32 1) || exit $?
expire=$(( $(date +%s) + validity_min * 60 ))

mkdir -p auth/ || exit $?
touch auth/index.html || exit $? # prevent directory listing of pending links
find auth/ -name '*.php' -mmin +$validity_min -delete # purge unused links

cat > "auth/$token.php" <<EOS
<?php
unlink(__FILE__); // one-time link

if (time() > $expire) {
    http_response_code(410);
    die('link expired');
}

require_once __DIR__ . '/../wp-load.php';

wp_set_auth_cookie($user_id);
header('Location: ' . parse_url(get_admin_url(), PHP_URL_PATH));
EOS
[ $? -eq 0 ] || exit $?

echo "${echo_prefix}https://$_DOMAIN_FIRST/auth/$token.php"
