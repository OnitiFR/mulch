#!/bin/bash

# -- Run with sudo privileges
# For: CentOS 7

sudo yum -y install mc mlocate man || exit $?

# Fix SSL confirmation issue with old pip (8.1.2 on CentOS 7)
# TODO: check if it's still needed
echo "WARNING: disabling SSL verification for pip"
sudo bash -c "cat > /etc/pip.conf" <<- EOS
[global]
trusted-host = pypi.python.org pypi.org files.pythonhosted.org
EOS
[ $? -eq 0 ] || exit $?

# Powerline
sudo yum -y install epel-release || exit $?
sudo yum -y install python-pip python-pygit2 || exit $?
sudo pip install powerline-status || exit $?
