#!/bin/bash

. ~/env

cd "$HTML_DIR" || exit $?

tar cf "$_BACKUP/app.tar" .
exitcode=$?

# If tar was given one of the --create, --append or --update
# options, this exit code (1) means that some files were changed
# while being archived and so the resulting archive does not contain
# the exact copy of the file set.
#
# NB: it happens a lot with Wordpress, since wp-content mtime is updated
# at every request…
if [ "$exitcode" != "1" ] && [ "$exitcode" != "0" ]; then
    exit $exitcode
fi

# password on the command line? brrr…
mysqldump -u $MYSQL_USER -h $MYSQL_HOST "-p$MYSQL_PASSWORD" $MYSQL_DB > "$_BACKUP/app.sql" || exit $?
