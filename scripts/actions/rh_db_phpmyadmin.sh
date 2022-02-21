#!/bin/bash

# Run as admin user

# 'do action' script for phpMyAdmin auto-login on rh-lamp.sh based VM

gentoken() {
    strings /dev/urandom | grep -o '[[:alnum:]]' | head -n 32 | tr -d '\n'; echo
}

dest_dir="/usr/local/lib/phpMyAdmin/login"
filename="$(gentoken).php" || exit $?
fullname="$dest_dir/$filename"

eval $(sudo grep MYSQL_PASSWORD /home/app/env)

if [ ! -d "$dest_dir" ]; then
    sudo mkdir "$dest_dir" || exit $?
fi
sudo chown $USER:app "$dest_dir" || exit $?
sudo chmod 770 "$dest_dir" || exit $?

# remove any older "session"
rm -f $dest_dir/*

cat > $fullname <<- EOS
<form method="post" action="../" id="go">
    <input type="hidden" name="pma_username" value="app">
    <input type="hidden" name="pma_password" value="$MYSQL_PASSWORD">
</form>
<script>document.getElementById('go').submit();</script>
<?php unlink(__FILE__); ?>
EOS
[ $? -eq 0 ] || exit $?

echo "_MULCH_OPEN_URL=https://$_DOMAIN_FIRST/_sql/login/$filename"

if [ "$(is_locked)" = true ]; then
    echo -e "This VM is \e[41mlocked\e[0m."
fi
