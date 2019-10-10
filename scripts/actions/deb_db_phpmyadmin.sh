#!/bin/bash

# Run as admin user

# 'do action' script for phpMyAdmin auto-login on deb-lamp.sh based VM

dest_dir="/usr/share/phpmyadmin/login"
filename="$(pwgen 32).php" || exit $?
fullname="$dest_dir/$filename"

eval $(sudo grep MYSQL_PASSWORD /home/$_APP_USER/env)

if [ ! -d "$dest_dir" ]; then
    sudo mkdir "$dest_dir" || exit $?
fi
sudo chown $USER:$_APP_USER "$dest_dir" || exit $?
sudo chmod 770 "$dest_dir" || exit $?

# remove any older "session"
rm -f $dest_dir/*

if [ -f /usr/share/phpmyadmin/auth.html ]; then
    # new method
    cat > $fullname <<- EOS
<?php
ini_set('session.use_cookies', 'true');
session_set_cookie_params(0, '/', '', true, true);
session_name('SignonSession');

@session_start();

\$_SESSION['PMA_single_signon_user'] = '$_APP_USER';
\$_SESSION['PMA_single_signon_password'] = '$MYSQL_PASSWORD';

@session_write_close();
unlink(__FILE__);
header('Location: ../index.php');
EOS
    [ $? -eq 0 ] || exit $?
else
    # old method
    cat > $fullname <<- EOS
<form method="post" action="../" id="go">
    <input type="hidden" name="pma_username" value="$_APP_USER">
    <input type="hidden" name="pma_password" value="$MYSQL_PASSWORD">
</form>
<script>document.getElementById('go').submit();</script>
<?php unlink(__FILE__); ?>
EOS
    [ $? -eq 0 ] || exit $?
fi

echo "_MULCH_OPEN_URL=https://$_DOMAIN_FIRST/_sql/login/$filename"
