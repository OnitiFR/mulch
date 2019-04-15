#!/bin/bash

# -- Run as app user

echo "downloading and installing wp-cli"
mkdir -p bin || exit $?
curl -so bin/wp -fSL "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar" || exit $?
chmod +x bin/wp || exit $?
