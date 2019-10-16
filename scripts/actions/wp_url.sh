#!/bin/bash

set -a
. ~/env
set +a

cd $HTML_DIR || exit $?

url="https://$_DOMAIN_FIRST"

wp option update home "$url" || exit $?
wp option update siteurl "$url" || exit $?

echo "* siteurl and home updated to $url"
