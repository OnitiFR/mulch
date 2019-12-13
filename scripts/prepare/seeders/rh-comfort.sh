#!/bin/bash

# -- Run with sudo privileges
# For: CentOS 7

sudo yum -y install mc mlocate man || exit $?

# Powerline
sudo yum -y install epel-release || exit $?
sudo yum -y install python-pip python-pygit2 || exit $?
sudo pip install powerline-status || exit $?
