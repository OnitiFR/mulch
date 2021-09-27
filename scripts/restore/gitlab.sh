#!/bin/bash

# -- Run with admin privileges

echo "copying backup…"

sudo cp $_BACKUP/mulch_gitlab_backup.tar /var/opt/gitlab/backups/ || exit $?
sudo chown git.git /var/opt/gitlab/backups/mulch_gitlab_backup.tar || exit $?

sudo cp $_BACKUP/gitlab-secrets.json /etc/gitlab/ || exit $?
sudo cp $_BACKUP/gitlab.rb /etc/gitlab/ || exit $?

# TODO: erase
# external_url 'https://xxx'
# gitlab_rails['gitlab_ssh_host'] = 'xxx'

sudo gitlab-ctl stop puma || exit $?
sudo gitlab-ctl stop sidekiq || exit $?

echo "backup restore…"

sudo gitlab-backup restore BACKUP=mulch force=yes || exit $?

sudo gitlab-ctl reconfigure || exit $?
sudo gitlab-ctl restart || exit $?
sudo gitlab-rake gitlab:check SANITIZE=true || exit $?
