#!/bin/bash

dest="/mnt/backup"

mkdir "$dest/vm"
cp /etc/mulch.env "$dest/vm"
/usr/local/bin/phone_home > "$dest/vm/vm-config.toml"
