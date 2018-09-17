#!/bin/bash

. /etc/mulch.env
. ~/env

cd "$HTML_DIR" || exit $?
tar cf "$_BACKUP/wordpress.tar" . || exit $?

# password on the command line? brrr…
mysqldump -u $MYSQL_USER -h $MYSQL_HOST "-p$MYSQL_PASSWORD" $MYSQL_DB > "$_BACKUP/wordpress.sql" || exit $?
