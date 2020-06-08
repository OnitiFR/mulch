#!/bin/bash

set -a
. ~/env
set +a

cd $HTML_DIR || exit $?

url="https://$_DOMAIN_FIRST"
old=$(wp option get home)

if [ "$old" = "$url" ]; then
    echo "Wordpress URL unchanged: $url"
    exit 0
fi

wp option update home "$url" || exit $?
wp option update siteurl "$url" || exit $?

echo "* siteurl and home updated to $url"

if [ $1 == 'with-content' ]; then
    echo "Updating content:"
    wp search-replace '$old' '$url' --skip-columns=guid || exit $?
else
    echo ""
    echo "You should now update URL in content."
    echo "minimal example:"
    echo "  wp search-replace '$old' '$url' --skip-columns=guid"
    echo "(next time, add 'with-content' argument, and I will do this automatically)"
    echo ""
    echo "See also extensions like 'Velvet Blues Update URLs'."
fi
