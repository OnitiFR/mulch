#!/bin/bash

# Run as admin user

arg1="$1"

if [ "$arg1" == "help" || "$arg1" == "--help" || "$arg1" == "-h" ]; then
  echo "Usage: $0 [access|error]"
  exit 0
fi

case "$arg1" in
  "access")
    sudo tail -f /var/log/apache2/access.log
    exit 0
    ;;
  "error")
    sudo tail -f /var/log/apache2/error.log
    exit 0
    ;;
  *)
    sudo tail -f /var/log/apache2/access.log /var/log/apache2/error.log
    ;;
esac
