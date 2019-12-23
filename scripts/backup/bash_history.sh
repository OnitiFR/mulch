#!/bin/bash

# -- Run as any user

if [ ! -f "$HOME/.bash_history"]; then
    # nothing to save
    exit 0
fi

mkdir -p "$_BACKUP/home/$USER" || exit $?
cp -p "$HOME/.bash_history" "$_BACKUP/home/$USER/" || exit $?
