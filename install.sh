#!/bin/bash

# This script does all the small things needed to install mulchd and
# mulch-proxy on a Linux system. Have a look to main() function for
# important stuff if you need to create a package.

# defaults (see --help)
ETC="/etc/mulch"
VAR_DATA="/var/lib/mulch"
VAR_STORAGE="/srv/mulch"

function main() {
    parse_args "$@"

    check_noroot # show warning if UID 0

    is_dir_writable "$ETC"
    is_dir_writable "$VAR_DATA"
    is_dir_writable "$VAR_STORAGE"
}

function check() {
    if [ $1 -ne 0 ]; then
        echo "error, exiting"
        exit $1
    fi
}

function parse_args() {
    while (( "$#" )); do
        case $1 in
        -e|--etc)
            shift
            ETC="$1"
            shift
            ;;
        -d|--data)
            shift
            VAR_DATA="$1"
            shift
            ;;
        -s|--storage)
            shift
            VAR_STORAGE="$1"
            shift
            ;;
        -h|--help)
            echo "** Install mulchd and mulch-proxy. **"
            echo "Options and defaults:"
            echo "  --etc $ETC"
            echo "  --data $VAR_DATA"
            echo "  --storage $VAR_STORAGE"
            exit 1
            ;;
        "")
        ;;
        *)
            echo "Unknown option $1"
            exit 2
            ;;
        esac
    done
}

function is_dir_writable() {
    echo "checking if $1 is writable…"
    if [ ! -d "$1" ]; then
        echo "error: directory $1 does not exists"
        exit 10
    fi
    test_file="$1/.wtest"
    touch "$test_file"
    check $?
    rm -f "$test_file"
}

function check_noroot() {
    uid=$(id -u)
    if [ "$uid" -eq 0 ]; then
        echo "ROOT PRIVILEGES NOT REQUIRED!"
    fi
}

main "$@"

# go install ./cmd... ?
# generate ssh key (if needed?)
# copy binaries?
# copy etc/ with templates (ex: /etc/mulch)
    # do not overwrite (in this case, warn the user)
# create services?
# API key? (generate a new one?)
# check storage accessibility (minimum: --x) for libvirt
# check user privileges about libvirt (= is in libvirt group?)
# check if libvirt is running?
# → for last two checks: virsh -c qemu:///system

# - check that your user is in `libvirt` group
#    - some distributions do this automatically on package install
#    - you may have to disconnect / reconnect your user
#    - if needed: `usermod -aG libvirt $USER`
