#!/bin/bash

# Run as admin user

arg1="$1"

if [ "$arg1" == "help" ] || [ "$arg1" == "--help" ] || [ "$arg1" == "-h" ]; then
  echo "Usage: [access|error]"
  exit 0
fi

case "$arg1" in
  "access")
    sudo tail -f /var/log/httpd/access_log
    exit 0
    ;;
  "error")
    sudo tail -f /var/log/httpd/error_log
    exit 0
    ;;
  "")
    sudo tail -f /var/log/httpd/access_log /var/log/httpd/error_log
    exit 0
    ;;
  *)
    echo "Usage: [access|error]" >&2
    exit 1
    ;;
esac
