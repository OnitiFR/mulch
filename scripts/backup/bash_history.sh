#!/bin/bash

# -- Run as any user

if [ ! -f "$HOME/.bash_history" ]; then
    # nothing to save
    exit 0
fi

cp -p "$HOME/.bash_history" "$_BACKUP/bash_history-$USER" || exit $?
