#!/bin/bash

# -- Run as app user

if [ -x bin/wp ]; then
  echo "wp-cli is already here"
else
  echo "downloading and installing wp-cli"
  mkdir -p bin || exit $?
  curl -so bin/wp -fSL "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar" || exit $?
  chmod +x bin/wp || exit $?
fi

echo "_MULCH_ACTION_NAME=update"
echo "_MULCH_ACTION_SCRIPT={core}/actions/wp_update.sh"
echo "_MULCH_ACTION_USER=$_APP_USER"
echo "_MULCH_ACTION_DESCRIPTION=Update Wordpress (core, themes, plugins, languages)"
echo "_MULCH_ACTION=commit"

echo "_MULCH_ACTION_NAME=url-reset"
echo "_MULCH_ACTION_SCRIPT={core}/actions/wp_url.sh"
echo "_MULCH_ACTION_USER=$_APP_USER"
echo "_MULCH_ACTION_DESCRIPTION=Reset siteurl+home settings using first VM domain name (argument 'with-content' available)"
echo "_MULCH_ACTION=commit"

echo "_MULCH_ACTION_NAME=login"
echo "_MULCH_ACTION_SCRIPT={core}/actions/wp_login.sh"
echo "_MULCH_ACTION_USER=$_APP_USER"
echo "_MULCH_ACTION_DESCRIPTION=Login to WP dashboard"
echo "_MULCH_ACTION=commit"

echo "_MULCH_ACTION_NAME=create-admin"
echo "_MULCH_ACTION_SCRIPT={core}/actions/wp_create_admin.sh"
echo "_MULCH_ACTION_USER=$_APP_USER"
echo "_MULCH_ACTION_DESCRIPTION=Create an WP admin user"
echo "_MULCH_ACTION=commit"

echo "_MULCH_TAG_ADD=wp-cli"
