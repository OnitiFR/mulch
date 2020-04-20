#!/bin/bash

set -a
. ~/env
set +a

# we will update themes as admin (need for some "premium" themes)
tmpfile=$(mktemp /tmp/wp-update.XXXXXX)
trap 'rm -f -- "$tmpfile"' INT TERM HUP EXIT
echo '<?php define( "WP_ADMIN", true );' > $tmpfile

cd $HTML_DIR || exit $?

wp core update || exit $?
wp plugin update --all || exit $?
wp theme update --all --require="$tmpfile" || exit $?
wp language core update || exit $?
wp language plugin update --all || exit $?
wp language theme update --all || exit $?

echo "* update completed, see https://$_DOMAIN_FIRST"
