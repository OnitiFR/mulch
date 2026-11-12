package topics

const (
	bashCompletionFunc = `
__mulch_get_server() {
    __mulch_current_server=$($COMP_LINE --dump-server)
}

__internal_list_toml_files() {
    local cur=${COMP_WORDS[COMP_CWORD]}

    local IFS=$'\n'
    COMPREPLY=( $( compgen -f -X '!*.toml' -- $cur ) )
}

__internal_list_qcow2_files() {
    local cur=${COMP_WORDS[COMP_CWORD]}

    local IFS=$'\n'
    COMPREPLY=( $( compgen -f -X '!*.qcow2' -- $cur ) )
}

__internal_list_backups() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server backup list --basic 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__internal_list_vms() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server vm list --basic 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__internal_list_actions() {
    local mulch_output out vm_name
    vm_name=$1
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server do $vm_name --basic 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__internal_list_seeds() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server seed list --basic 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__internal_doaction() {
	local prev_prev=${COMP_WORDS[COMP_CWORD-2]}
    if [ "$prev" =  "do" ]; then
        __internal_list_vms
    elif [ "$prev_prev" =  "do" ]; then
        __internal_list_actions $prev
    fi
}

__internal_list_keys() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server key list --basic 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

# completion of "<subcommand> <arg1> [arg2]" commands, by position
# (key ssh …, trust …). Words are read keeping ':' (fingerprints contain
# ':', a word break for bash), using bash-completion helpers when available.
# $1: arg1 completion function, $2: arg2 completion function (called with
# arg1 and the current word), $3…: subcommand name and its aliases
__internal_two_args() {
    local arg1_func=$1 arg2_func=$2 name
    shift 2
    local c_cur=$cur c_prev=$prev c_words c_cword
    c_words=("${COMP_WORDS[@]}")
    c_cword=$COMP_CWORD
    if declare -F _get_comp_words_by_ref >/dev/null; then
        _get_comp_words_by_ref -n : -c c_cur -p c_prev -w c_words -i c_cword
    fi

    for name in "$@"; do
        if [ "$c_prev" = "$name" ]; then
            $arg1_func
            return
        fi
    done
    for name in "$@"; do
        if [ "${c_words[c_cword-2]}" = "$name" ]; then
            $arg2_func "$c_prev" "$c_cur"
            return
        fi
    done
}

__internal_none() {
    :
}

# *.pub files, and directories to reach them (ex: ~/.ssh/)
__internal_pub_files() {
    local IFS=$'\n'
    compopt -o filenames 2>/dev/null
    COMPREPLY=( $( compgen -f -X '!*.pub' -- "$2"; compgen -d -- "$2" ) )
}

__internal_on_off() {
    COMPREPLY=( $( compgen -W "on off" -- "$2" ) )
}

# $1: current word, $2…: mulch command listing fingerprints (first column)
__internal_fingerprints() {
    local c_cur=$1 mulch_output
    shift
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server "$@" 2>/dev/null); then
        COMPREPLY=( $( compgen -W "$(echo "${mulch_output}" | awk '{print $1}')" -- "$c_cur" ) )
        if declare -F __ltrim_colon_completions >/dev/null; then
            __ltrim_colon_completions "$c_cur"
        fi
    fi
}

__internal_key_fingerprints() {
    __internal_fingerprints "$2" key ssh list "$1"
}

__internal_trust_fingerprints() {
    __internal_fingerprints "$2" trust list "$1" --basic
}

__internal_list_peers() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server peer list 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__internal_list_secrets() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server secret list --basic 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__internal_list_greenhouse_vms() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server greenhouse list --basic 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__mulch_get_servers() {
    local out servers
    servers=$(grep -E '^[[:blank:]]*name[[:blank:]]*=' ~/.mulch.toml | awk -F= '{print $2}')
    out=($(echo $servers))
    COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
}

__internal_migrate() {
	local prev_prev=${COMP_WORDS[COMP_CWORD-2]}
    if [ "$prev" =  "migrate" ]; then
        __internal_list_vms
    elif [ "$prev_prev" =  "migrate" ]; then
        __internal_list_peers
    fi
}

__mulch_custom_func() {
    case ${last_command} in
        mulch_vm_create)
            __internal_list_toml_files
            return
            ;;
        mulch_ssh | mulch_vm_backup | mulch_vm_config | mulch_vm_delete | mulch_vm_infos | mulch_vm_lock | mulch_vm_rebuild | mulch_vm_redefine | mulch_vm_start | mulch_vm_stop | mulch_vm_unlock | mulch_vm_activate | mulch_vm_deactivate | mulch_log | mulch_vm_console | mulch_vm_restart | mulch_vm_load)
            __internal_list_vms
            return
            ;;
        mulch_vm_abort)
            __internal_list_greenhouse_vms
            return
            ;;
        mulch_backup_cat | mulch_backup_delete | mulch_backup_download | mulch_backup_expire)
            __internal_list_backups
            return
            ;;
        mulch_backup_upload)
            __internal_list_qcow2_files
            return
            ;;
        mulch_do)
            __internal_doaction
            return
            ;;
            mulch_vm_migrate)
            __internal_migrate
            return
            ;;
        mulch_seed_status | mulch_seed_refresh)
            __internal_list_seeds
            return
            ;;
        mulch_key_right_list | mulch_key_right_add | mulch_key_right_remove | mulch_key_delete)
            __internal_list_keys
            return
            ;;
        mulch_key_ssh_list)
            __internal_two_args __internal_list_keys __internal_none list
            return
            ;;
        mulch_key_ssh_add)
            __internal_two_args __internal_list_keys __internal_pub_files add
            return
            ;;
        mulch_key_ssh_remove)
            __internal_two_args __internal_list_keys __internal_key_fingerprints remove
            return
            ;;
        mulch_key_ssh_only)
            __internal_two_args __internal_list_keys __internal_on_off only
            return
            ;;
        mulch_trust_list)
            __internal_two_args __internal_list_vms __internal_none list
            return
            ;;
        mulch_trust_forward)
            __internal_two_args __internal_list_vms __internal_pub_files forward add
            return
            ;;
        mulch_trust_remove)
            __internal_two_args __internal_list_vms __internal_trust_fingerprints remove delete unforward
            return
            ;;
        mulch_secret_set | mulch_secret_get | mulch_secret_delete | mulch_secret_list)
            __internal_list_secrets
            return
            ;;
        *)
            ;;
    esac
}
`
)
