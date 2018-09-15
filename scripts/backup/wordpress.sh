#!/bin/bash

. ~/env

dest="/mnt/backup"

cd "$HTML_DIR" || exit $?
tar cf $dest/wordpress.tar . || exit $?

# password on the command line? brrr…
mysqldump -u $MYSQL_USER -h $MYSQL_HOST "-p$MYSQL_PASSWORD" $MYSQL_DB > "$dest/wordpress.sql" || exit $?
