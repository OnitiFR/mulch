#!/bin/bash

set -a
. ~/env
set +a

cd $HTML_DIR || exit $?

if [ -n "$1" ]; then
    user_login="$1"
else
    user_login="$_CALLING_KEY"
fi

if [ -n "$2" ]; then
    user_email="$2"
else
    set -o pipefail
    domain=$(wp option get admin_email | cut -d@ -f2)
    if [ $? -ne 0 ]; then
        >&2 echo "enable to get admin email (wp-cli)"
        exit 1
    fi
    set +o pipefail
    echo "using '$domain', based on admin email"
    user_email="$_CALLING_KEY@$domain"
fi

if [ -z "$user_login" ]; then
    >&2 echo "you must provide a username as first argument"
    exit 1
fi

if [ -z "$user_email" ]; then
    >&2 echo "you must provide an email as second argument"
    exit 1
fi

echo "note: you can provide <username> <email> arguments"
echo "creating admin user '$user_login' with address '$user_email'"

wp user create "$user_login" "$user_email" --role=administrator || exit $?
