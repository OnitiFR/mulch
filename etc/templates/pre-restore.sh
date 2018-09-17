#!/bin/bash

. /etc/mulch.env

part="/dev/disk/by-label/backup"

tests=0
while [ ! -e "$part" ]; do
    echo "waiting backup disk…"
    tests=$[$tests+1]
    sleep 1

    if [ $tests -eq 5 ]; then
        >&2 echo "can't find backup disk $part"
        exit 10
    fi
done

sudo mkdir -p "$_BACKUP" || exit $?
sudo mount "$part" "$_BACKUP" || exit $?
sudo chmod 0777 "$_BACKUP" || exit $?
