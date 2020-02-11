#!/bin/bash

# -- Run with sudo privileges
# For: Debian 9+ / Ubuntu 18.10+

export DEBIAN_FRONTEND="noninteractive"
sudo -E apt-get -y -qq install progress mc powerline locate man || exit $?

# powerline-gitstatus for Ubuntu >= 18.10
available=$(sudo apt-cache search --names-only '^powerline-gitstatus$' | wc -l)
if [ $available -gt 0 ]; then
    sudo -E apt-get -y -qq install powerline-gitstatus || exit $?
fi
