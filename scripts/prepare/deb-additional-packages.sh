#!/bin/bash

# -- run as admin
# fill ADDITIONAL_PACKAGES env var

export DEBIAN_FRONTEND="noninteractive"

sudo -E apt-get -y -qq install $ADDITIONAL_PACKAGES || exit $?
