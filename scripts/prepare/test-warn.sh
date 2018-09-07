#!/bin/bash

# -- Run with sudo privileges

sudo bash -c "cat >> /etc/motd" <<- EOS
-- THIS IS A TEST VM --
For production use, remove dev.sh script AND ENABLE init_upgrade setting!

EOS
