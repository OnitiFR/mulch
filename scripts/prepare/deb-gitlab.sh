#!/bin/bash

# Install GitLab CE
# You can define GITLAB_VERSION env if needed.
# -- Run as admin user
# Straight from https://about.gitlab.com/install/#ubuntu

curl -s https://packages.gitlab.com/install/repositories/gitlab/gitlab-ce/script.deb.sh | sudo bash || exit $?

export EXTERNAL_URL="https://$_DOMAIN_FIRST"
export GITLAB_OMNIBUS_CONFIG="letsencrypt['enable'] = false"

if [ -n "$GITLAB_VERSION" ]; then
    sudo --preserve-env=EXTERNAL_URL,GITLAB_OMNIBUS_CONFIG apt-get -y -qq install "gitlab-ce=$GITLAB_VERSION" || exit $?
else
    sudo --preserve-env=EXTERNAL_URL,GITLAB_OMNIBUS_CONFIG apt-get -y -qq install gitlab-ce || exit $?
fi
