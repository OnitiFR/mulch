#!/bin/bash

# -- Run with admin privileges

sudo mkdir -p /var/opt/gitlab/backups/ || exit $?

sudo cp $_BACKUP/mulch_gitlab_backup.tar /var/opt/gitlab/backups/ || exit $?
sudo chown git.git /var/opt/gitlab/backups/mulch_gitlab_backup.tar || exit $?

sudo cp $_BACKUP/gitlab-secrets.json /etc/gitlab/ || exit $?
sudo cp $_BACKUP/gitlab.rb /etc/gitlab/ || exit $?

sudo gitlab-ctl stop puma || exit $?
sudo gitlab-ctl stop sidekiq || exit $?

sudo gitlab-backup restore BACKUP=mulch || exit $?

sudo gitlab-ctl reconfigure || exit $?
sudo gitlab-ctl restart || exit $?
sudo gitlab-rake gitlab:check SANITIZE=true || exit $?
