#!/bin/bash

# This script can be used to remotly backup a Mulch hosted VM.
# From another system, it will backup the VM, download the resulting file
# and delete the remote backup on the Mulch server.

# This script remove local backups older than 10 days.

# Required key rights:
# GET /vm/infos/*
# POST /vm/* action=backup
# GET /backup/*
# DELETE /backup/*

if [ -z "$2" ]; then
    (>&2 echo "ERROR: give vm-name and destination backup-dir")
    (>&2 echo "Usage example: $0 my-vm /home/backups/data")
    exit 1
fi

vm_name=$1
backup_dir=$2

# $1: error code
# $2: reason message
function check() {
if [ $1 -ne 0 ]; then
    (>&2 echo "ERROR: $vm_name: $2")
    exit 1
fi
}

## Checks

mulch vm infos "$vm_name" > /dev/null
check $? "unable to check VM"

cd "$backup_dir"
check $? "unable to check backup disk"

## Actual backup

data=$(mulch vm backup "$vm_name")
check $? "unable to backup"

backup_name=$(echo "$data" | grep BACKUP= | cut -d= -f2)

mulch backup download "$backup_name" > /dev/null
check $? "unable to download backup"

## Cleaning

mulch backup delete "$backup_name" > /dev/null
check $? "unable to delete backup"

find "$backup_dir" -type f -mtime +10 -delete
check $? "unable to delete old backups"
