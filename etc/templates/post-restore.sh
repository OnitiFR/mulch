#!/bin/bash

. /etc/mulch.env

sudo umount "$_BACKUP" || exit $?
