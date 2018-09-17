#!/bin/bash

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

# create temporary handle
tmpfile=$(mktemp)
rm "$tmpfile"

echo "resizing FS $part…"
sudo resize2fs "$part" > "$tmpfile" 2>&1
if [ $? -ne 0 ]; then
    cat "$tmpfile"
    rm "$tmpfile"
    exit 99
fi

sudo mkdir -p /mnt/backup || exit $?
sudo mount "$part" /mnt/backup || exit $?
sudo chmod 0777 /mnt/backup || exit $?

rm "$tmpfile"
