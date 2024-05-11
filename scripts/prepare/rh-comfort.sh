#!/bin/bash

# -- Run with sudo privileges
# For: CentOS 7

sudo yum -y install mc mlocate man || exit $?

sudo bash -c "cat > /usr/share/mc/mc.ini" <<- EOS
[Midnight-Commander]
use_internal_edit=1
editor_edit_confirm_save=0
confirm_exit=0

[Panels]
navigate_with_arrows=1
EOS
[ $? -eq 0 ] || exit $?


sualias="$_APP_USER"
rh=$(cat /etc/redhat-release)
sudo bash -c "cat > /etc/motd" <<- EOS

$rh: $_VM_NAME (rev $_VM_REVISION)
    Generated by Mulch $_MULCH_VERSION on $_VM_INIT_DATE by $_KEY_DESC
    See "mulch ssh" command to connect with another user
    With user $_MULCH_SUPER_USER, you can use '$sualias' and 'root' commands

EOS
[ $? -eq 0 ] || exit $?

sudo bash -c "cat > /etc/profile.d/mulch.sh" <<- EOS
if ! shopt -oq posix; then
  alias $sualias="sudo -iu $_APP_USER"
  alias root="sudo -i"
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
if [ "x\${BASH_VERSION-}" != x -a "x\${PS1-}" != x ]; then
  if [ "\$(/usr/local/bin/is_locked)" = true ]; then
    echo -e "This VM is \e[41mlocked\e[0m."
  fi
fi
EOS

# Fix SSL confirmation issue with old pip (8.1.2 on CentOS 7)
# TODO: check if it's still needed
sudo bash -c "cat > /etc/pip.conf" <<- EOS
[global]
trusted-host = pypi.python.org pypi.org files.pythonhosted.org
EOS
[ $? -eq 0 ] || exit $?

# Powerline
sudo yum -y install epel-release || exit $?
sudo yum -y install python-pip python-pygit2 || exit $?
sudo pip install powerline-status || exit $?

sudo bash -c "cat > /etc/profile.d/powerline.sh" <<- 'EOS'
if ! shopt -oq posix; then
  powerline_sh=$(find /usr/lib -name powerline.sh |grep bash)
  if [ -f "$powerline_sh" ]; then
    . "$powerline_sh"
  fi
fi
EOS
[ $? -eq 0 ] || exit $?

# add powerline vcs.branch before shell.cwd + show VM name instead of hostname
theme="/usr/lib/python2.7/site-packages/powerline/config_files/themes/shell/default.json"
line=$(grep -n powerline.segments.shell.cwd "$theme" | cut -d: -f1)
line=$(expr $line - 2)
sudo sed -i "$line a {\"function\": \"powerline.segments.common.vcs.branch\", \"priority\": 40, \"args\": {\"status_colors\": false}}," "$theme" || exit $?
sudo sed -i "s/\"function\": \"powerline.segments.common.net.hostname\",/\"function\": \"powerline.segments.common.env.environment\", \"args\": {\"variable\": \"_VM_NAME\"},/" "$theme" || exit $?

# add a "open" action (see "do" command) if there's any domain defined
if [ -n "$_DOMAIN_FIRST" ]; then
    echo "_MULCH_ACTION_NAME=open"
    echo "_MULCH_ACTION_SCRIPT={core}/actions/open.sh"
    echo "_MULCH_ACTION_USER=$_APP_USER"
    echo "_MULCH_ACTION_DESCRIPTION=Open VM first domain in the browser"
    echo "_MULCH_ACTION=commit"
fi
