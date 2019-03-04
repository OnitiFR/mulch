#!/bin/bash

# -- Run as app user
# Warning: will fail if no crontab was saved

crontab "$_BACKUP/crontab" || exit $?
