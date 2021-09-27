#!/bin/bash

# -- Run as admin user

sudo gitlab-backup create BACKUP=mulch || exit $?

echo "copying backup…"
sudo mv /var/opt/gitlab/backups/mulch_gitlab_backup.tar $_BACKUP || exit $?

sudo cp /etc/gitlab/gitlab-secrets.json $_BACKUP || exit $?
sudo cp /etc/gitlab/gitlab.rb $_BACKUP || exit $?
