#!/bin/bash

# -- Run as any user

if [ -f "$_BACKUP/home/$USER/.bash_history" ]; then
    cp -p "$_BACKUP/home/$USER/.bash_history" "$HOME" || exit $?
fi
