#!/bin/bash

# This script does all the small things needed to install mulchd and
# mulch-proxy on a Linux system. Have a look to main() function for
# important stuff if you need to create a package.

# See also install/ directory for basic/demo installations.

# defaults (see --help)
ETC="/etc/mulch"
VAR_DATA="/var/lib/mulch"
VAR_STORAGE="/srv/mulch"
FORCE="false"

SOURCE=$(dirname "$0")

# TODO:
# check storage accessibility (minimum: --x) for libvirt? (RH/Fedora)
#   setfacl -m g:qemu:x /home/mulch/
#   setfacl -m g:libvirt-qemu:x /home/mulch/

function main() {
    parse_args "$@"

    check_noroot # show warning if UID 0

    check_libvirt_access

    is_dir_writable "$ETC"
    is_dir_writable "$VAR_DATA"
    is_dir_writable "$VAR_STORAGE"

    check_if_existing_config

    copy_config
    update_config_path
    gen_services

    infos_next
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
        -f|--force)
            FORCE="true"
            shift
            ;;
        -h|--help)
            echo ""
            echo "** Helper script: install mulchd and mulch-proxy **"
            echo ""
            echo "Note: mulch client is not installed/configured by this script."
            echo ""
            echo "Options and defaults (short options available too):"
            echo "  --etc $ETC (-e, config files)"
            echo "  --data $VAR_DATA (-d, state [small] databases)"
            echo "  --storage $VAR_STORAGE (-s, disks storage)"
            echo "  --force (-f, erase old install)"
            echo ""
            echo "Sample install:"
            echo "mkdir -p ~/mulch/etc ~/mulch/data ~/mulch/storage"
            echo "./install.sh --etc ~/mulch/etc/ --data ~/mulch/data/ --storage ~/mulch/storage/"
            echo ""
            echo "For a quick demo install for Ubuntu, see install/ubuntu_autoinstall.sh"
            exit 1
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

function check_if_existing_config() {
    if [ -f "$ETC/mulchd.toml" ]; then
        echo "Existing configuration found!"
        if [ $FORCE == "false" ]; then
            echo "This script is intentend to do a new install, not to upgrade an existing one."
            echo "If you know what you are doing, you may use --force option."
            echo "Exiting."
            exit 1
        fi
    fi
}

function copy_config() {
    echo "copying config…"
    cp -Rp $SOURCE/etc/* "$ETC/"
    check $?
    mv "$ETC/mulchd.sample.toml" "$ETC/mulchd.toml"
    check $?
}

function check_libvirt_access() {
    echo "checking libvirt access…"
    virsh -c qemu:///system version
    ret=$?
    if [ "$ret" -ne 0 ]; then
        echo "Failed."
        echo " - check that libvirtd is running:"
        echo "   - systemctl status libvirtd"
        echo " - check that $USER is allowed to connect to qemu:///system URL:"
        echo "   - check that your user is in 'libvirt' group"
        echo "   - sudo usermod -aG libvirt \$USER"
        echo "   - you may have to disconnect / reconnect your user"
    fi
    check $ret
}

function update_config_path() {
    sed -i'' "s|^data_path =.*|data_path = \"$VAR_DATA\"|" "$ETC/mulchd.toml"
    check $?
    sed -i'' "s|^storage_path =.*|storage_path = \"$VAR_STORAGE\"|" "$ETC/mulchd.toml"
    check $?
}

function infos_next() {
    echo ""
    echo "Install OK."
    echo ""
    echo "Now, you can:"
    echo " - update $ETC/mulchd.toml"
    echo " - test manually mulchd and mulch-proxy"
    echo "   - $mulchd_bin -path \"$ETC\""
    echo "   - $proxy_bin -path \"$ETC\""
    echo " - install+start services (root)"
    echo "   - sudo cp mulchd.service mulch-proxy.service /etc/systemd/system/ (no symlink)"
    echo "   - sudo systemctl daemon-reload"
    echo "   - sudo systemctl enable --now mulchd"
    echo "   - sudo systemctl enable --now mulch-proxy"
    echo " - get API key(s) in $VAR_DATA/mulch-api-keys.db"
    echo " - have fun with mulch client"
}

function gen_services() {
    echo "generating systemd unit service files…"
    go_bin=$(go env GOBIN)
    if [ -z "$go_bin" ]; then
        go_bin="$(go env GOPATH)/bin"
    fi

    # should apply systemd-escape ?
    mulchd_bin="$go_bin/mulchd"
    proxy_bin="$go_bin/mulch-proxy"

    if [ ! -x "$mulchd_bin" ]; then
        echo "Unable to find $mulchd_bin (compilation was OK?)"
        check 20
    fi

    if [ ! -x "$proxy_bin" ]; then
        echo "Unable to find $proxy_bin (compilation was OK?)"
        check 20
    fi

    cp -p "$SOURCE/install/mulchd.sample.service" "$SOURCE/mulchd.service"
    check $?
    cp -p "$SOURCE/install/mulch-proxy.sample.service" "$SOURCE/mulch-proxy.service"
    check $?

    sed -i'' "s|{USER}|$USER|" "$SOURCE/mulchd.service"
    check $?
    sed -i'' "s|{USER}|$USER|" "$SOURCE/mulch-proxy.service"
    check $?

    sed -i'' "s|{MULCHD_START}|$mulchd_bin -path \"$ETC\"|" "$SOURCE/mulchd.service"
    check $?
    sed -i'' "s|{MULCH_PROXY_START}|$proxy_bin -path \"$ETC\"|" "$SOURCE/mulch-proxy.service"
    check $?
    sed -i'' "s|{MULCH_PROXY}|$proxy_bin|" "$SOURCE/mulch-proxy.service"
    check $?
}

main "$@"
