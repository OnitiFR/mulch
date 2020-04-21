#!/bin/bash

set -a
. ~/env
set +a

# TODO: wp wc update? (if woocommerce detected)
# TODO: find a way to update things like Divi and other "premium" themes / plugins

cd $HTML_DIR || exit $?

wp core update || exit $?
wp core update-db || exit $?
wp plugin update --all || exit $?
wp theme update --all || exit $?
wp language core update || exit $?
wp language plugin update --all || exit $?
wp language theme update --all || exit $?

echo "* update completed, see https://$_DOMAIN_FIRST"
