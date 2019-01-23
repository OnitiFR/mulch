#!/bin/bash

# -- Run as app user
# Warning: will fail if no crontab is installed

crontab -l > "$_BACKUP/crontab" || exit $?
