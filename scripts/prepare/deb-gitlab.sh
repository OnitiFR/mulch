#!/bin/bash

# Install GitLab CE
# You can define GITLAB_VERSION env if needed.
# -- Run as admin user
# Straight from https://about.gitlab.com/install/#ubuntu

curl -s https://packages.gitlab.com/install/repositories/gitlab/gitlab-ce/script.deb.sh | sudo bash || exit $?

export EXTERNAL_URL="http://$_DOMAIN_FIRST"

if [ -n "$GITLAB_VERSION" ]; then
    sudo --preserve-env=EXTERNAL_URL apt-get -y -qq install "gitlab-ce=$GITLAB_VERSION" || exit $?
else
    sudo --preserve-env=EXTERNAL_URL apt-get -y -qq install gitlab-ce || exit $?
fi

# apply initial config

sudo sed -i "s|^external_url .*$|external_url 'https://$_DOMAIN_FIRST'|" /etc/gitlab/gitlab.rb || exit $?

sudo tee -a /etc/gitlab/gitlab.rb > /dev/null <<EOF
nginx['listen_port'] = 80
nginx['listen_https'] = false
nginx['real_ip_trusted_addresses'] = ['$_MULCH_PROXY_IP']
nginx['real_ip_header'] = 'X-Forwarded-For'
nginx['real_ip_recursive'] = 'on'
letsencrypt['enable'] = false
EOF

sudo gitlab-ctl reconfigure || exit $?
sudo gitlab-ctl restart || exit $?

echo
echo "--- GitLab CE installed ---"
echo "You can find the initial root password in /etc/gitlab/initial_root_password"
echo "Remember to change it after first login."
echo "---------------------------"

