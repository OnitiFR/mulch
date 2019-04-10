#!/bin/bash

# -- Run as app user

if [ -f "$_BACKUP/crontab" ]; then
    crontab "$_BACKUP/crontab" || exit $?
fi
