#!/bin/bash

# -- Run with sudo privileges

sudo bash -c "cat >> /etc/motd" <<- EOS
-- THIS IS A TEST VM --
For production use:
  - remove test-warn.sh prepare script
  - AND ENABLE init_upgrade setting!

EOS
