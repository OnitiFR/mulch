#!/bin/bash

# Tune OpenSSH server for a higher availability
# in public-facing environments

# -- Run as admin user

function comment_setting() {
    setting="$1"
    file="$2"
    sudo sed -e "/$setting/ s/^#*/#/" -i "$file" || exit ?
}

function add_line() {
    text="$1"
    file="$2"
    echo "$text" | sudo tee -a "$file" > /dev/null || exit ?
}

file="/etc/ssh/sshd_config"

comment_setting "LoginGraceTime" "$file"
comment_setting "MaxStartups" "$file"
comment_setting "MaxAuthTries" "$file"

file="/etc/ssh/sshd_config.d/10-deb-sshd-tune.conf"
sudo bash -c "cat > $file" <<- 'EOS'
# added by deb-sshd-tune.sh
LoginGraceTime 15
MaxStartups 50:30:120
MaxAuthTries 4
EOS
[ $? -eq 0 ] || exit $?

ssh_unit=$(systemctl list-unit-files | cut -d\  -f1 | grep 'sshd*\.service' | head -n1)

echo "Restarting $ssh_unit for new host keys"
sudo systemctl restart $ssh_unit || exit $?
