#!/bin/bash

# Generic LAMP restore script
# -- Run with app privileges

. ~/env

rm -rf "$HTML_DIR" || exit $?
mkdir "$HTML_DIR" || exit $?

cd "$HTML_DIR" || exit $?

tar xf "$_BACKUP/app.tar" || exit $?

# password on the command line? brrr…
mysql -u $MYSQL_USER -h $MYSQL_HOST "-p$MYSQL_PASSWORD" $MYSQL_DB < "$_BACKUP/app.sql" || exit $?
