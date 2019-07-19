#!/bin/bash

# -- Run as app user

echo "downloading and installing wp-cli"
mkdir -p bin || exit $?
curl -so bin/wp -fSL "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar" || exit $?
chmod +x bin/wp || exit $?

echo "_MULCH_ACTION_NAME=update"
echo "_MULCH_ACTION_SCRIPT=https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/actions/wp_update.sh"
echo "_MULCH_ACTION_USER=$_APP_USER"
echo "_MULCH_ACTION_DESCRIPTION=Update Wordpress (core, themes, plugins, languages)"
echo "_MULCH_ACTION=commit"
