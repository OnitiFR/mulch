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

# create temporary handle
tmpfile=$(mktemp)
rm "$tmpfile"

# now tries to create a new XFS instead of resizing the existing
# template ext2 FS. It's way faster on large volumes and do not implies
# big qcow2 files as a result.
sudo which mkfs.xfs > /dev/null
if [ $? -ne 0 ]; then
  echo "resizing FS on $part… (ext)"
  sudo resize2fs "$part" > "$tmpfile" 2>&1
  if [ $? -ne 0 ]; then
      cat "$tmpfile"
      rm "$tmpfile"
      exit 99
  fi
else
  echo "creating FS on $part… (xfs)"
  sudo mkfs.xfs -f -L backup "$part" > "$tmpfile" 2>&1
  if [ $? -ne 0 ]; then
      cat "$tmpfile"
      rm "$tmpfile"
      exit 99
  fi
fi

sudo mkdir -p "$_BACKUP" || exit $?
sudo mount "$part" "$_BACKUP" || exit $?
sudo chmod 0777 "$_BACKUP" || exit $?

mkdir "$_BACKUP/mulch"
cp /etc/mulch.env "$_BACKUP/mulch"
/usr/local/bin/phone_home > "$_BACKUP/mulch/vm-config.toml"

rm "$tmpfile"
