#!/bin/bash

set -a
. ~/env
set +a

cd $HTML_DIR || exit $?

wp core update || exit $?
wp plugin update --all || exit $?
wp theme update --all || exit $?
wp language core update || exit $?
wp language plugin update --all || exit $?
wp language theme update --all || exit $?

echo "* update completed, see https://$_DOMAIN_FIRST"
