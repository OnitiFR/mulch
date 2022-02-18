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

# Set Midnight-Commander as the default editor, and apply
# a cool setup to it
sudo update-alternatives --set editor /usr/bin/mcedit || exit $?
sudo bash -c "cat > /usr/share/mc/mc.ini" <<- EOS
[Midnight-Commander]
use_internal_edit=true
editor_edit_confirm_save=false

[Panels]
navigate_with_arrows=true
EOS
[ $? -eq 0 ] || exit $?

sualias="$_APP_USER"
sudo bash -c "cat > /etc/motd" <<- EOS

Debian GNU/Linux: $_VM_NAME (rev $_VM_REVISION)
    Generated by Mulch $_MULCH_VERSION on $_VM_INIT_DATE by $_KEY_DESC
    Switch to application user: sudo -iu $_APP_USER (alias: $sualias)
    Switch to other users: see "mulch ssh" command

EOS
[ $? -eq 0 ] || exit $?

sudo bash -c "cat > /etc/profile.d/mulch.sh" <<- EOS
if ! shopt -oq posix; then
  alias $sualias="sudo -iu $_APP_USER"
  alias e="mcedit"
  alias ll="ls -la --color"
  alias l="ls"

  alias gt="git status -s"
  alias gd="git diff"
  alias gp="git push"
  alias gl="git pull"
  alias ga="git add -A"
  alias gc="git commit -m"
fi
EOS
[ $? -eq 0 ] || exit $?

sudo bash -c "cat > /etc/profile.d/is_locked.sh" <<- EOS
if ! shopt -oq posix; then
  if [ "\$(is_locked)" = true ]; then
    echo -e "Warning: this VM is \e[41mlocked\e[0m!"
  fi
fi
EOS
[ $? -eq 0 ] || exit $?

# powerline: show VM name on prompt instead of hostname
themes="/usr/share/powerline/config_files/themes/shell/default*.json"
sudo sed -i "s/\"function\": \"powerline.segments.common.net.hostname\",/\"function\": \"powerline.segments.common.env.environment\", \"args\": {\"variable\": \"_VM_NAME\"},/" $themes || exit $?

sudo bash -c "cat > /etc/profile.d/powerline.sh" <<- EOS
if ! shopt -oq posix; then
  if [ -f /usr/share/powerline/bindings/bash/powerline.sh ]; then
    . /usr/share/powerline/bindings/bash/powerline.sh
  fi
fi
EOS
[ $? -eq 0 ] || exit $?

# remove public access for home directories
sudo chmod o= /home/*/ /etc/skel || exit $?

# add a "open" action (see "do" command) if there's any domain defined
if [ -n "$_DOMAIN_FIRST" ]; then
    echo "_MULCH_ACTION_NAME=open"
    echo "_MULCH_ACTION_SCRIPT=https://raw.githubusercontent.com/OnitiFR/mulch/master/scripts/actions/open.sh"
    echo "_MULCH_ACTION_USER=$_APP_USER"
    echo "_MULCH_ACTION_DESCRIPTION=Open VM first domain in the browser"
    echo "_MULCH_ACTION=commit"
fi
