#!/bin/bash

# -- Run as any user

if [ -f "$_BACKUP/bash_history-$USER" ]; then
    cp -p "$_BACKUP/bash_history-$USER" "$HOME/.bash_history" || exit $?
fi
