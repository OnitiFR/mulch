#!/bin/bash

# -- run as admin
# fill ADDITIONAL_PACKAGES env var

sudo yum -y install $ADDITIONAL_PACKAGES || exit $?
