#!/bin/bash

# Run as admin user

# 'do action' script for Adminer auto-login on deb-php-pgsql.sh based VM

dest_dir="/usr/share/adminer/login"
filename="$(pwgen 32).php" || exit $?
fullname="$dest_dir/$filename"

eval $(sudo grep PGSQL_PASSWORD /home/app/env)

if [ ! -d "$dest_dir" ]; then
    sudo mkdir "$dest_dir" || exit $?
fi
sudo chown $USER:app "$dest_dir" || exit $?
sudo chmod 770 "$dest_dir" || exit $?

# remove any older "session"
rm -f $dest_dir/*

cat > $fullname <<- EOS
<form method="post" action="../adminer.php" id="go">
    <input type="hidden" name="auth[driver]" value="pgsql">
    <input type="hidden" name="auth[server]" value="">
    <input type="hidden" name="auth[username]" value="app">
    <input type="hidden" name="auth[password]" value="$PGSQL_PASSWORD">
    <input type="hidden" name="auth[db]" value="app">
    <input type="hidden" name="auth[permanent]" value="1">
</form>
<script>document.getElementById('go').submit();</script>
<?php unlink(__FILE__); ?>
EOS
[ $? -eq 0 ] || exit $?

echo "_MULCH_OPEN_URL=https://$_DOMAIN_FIRST/_sql/login/$filename"

if [ "$(is_locked)" = true ]; then
    echo -e "This VM is \e[41mlocked\e[0m."
fi
