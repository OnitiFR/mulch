#!/bin/bash

. ~/env

cd "$HTML_DIR" || exit $?
tar cf "$_BACKUP/app.tar" . || exit $?

# password on the command line? brrr…
mysqldump -u $MYSQL_USER -h $MYSQL_HOST "-p$MYSQL_PASSWORD" $MYSQL_DB > "$_BACKUP/app.sql" || exit $?
