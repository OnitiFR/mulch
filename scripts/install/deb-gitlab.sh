#!/bin/bash

# Install GitLab CE
# You can define GITLAB_VERSION env if needed.
# -- Run as admin user
# Straight from https://about.gitlab.com/install/#ubuntu

curl -s https://packages.gitlab.com/install/repositories/gitlab/gitlab-ce/script.deb.sh | sudo bash || exit $?

if [ -n "$GITLAB_VERSION" ]; then
    sudo EXTERNAL_URL="http://$_DOMAIN_FIRST" apt-get -y -qq install "gitlab-ce=$GITLAB_VERSION" || exit $?
else
    sudo EXTERNAL_URL="http://$_DOMAIN_FIRST" apt-get -y -qq install gitlab-ce || exit $?
fi
