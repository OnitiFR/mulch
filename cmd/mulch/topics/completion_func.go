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

__internal_list_peers() {
    local mulch_output out
    __mulch_get_server
    if mulch_output=$(mulch --server $__mulch_current_server peer list 2>/dev/null); then
        out=($(echo "${mulch_output}"))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
}

__mulch_get_servers() {
    local out servers
    servers=$(egrep '^[[:blank:]]*name[[:blank:]]*=' ~/.mulch.toml | awk -F= '{print $2}')
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
        mulch_ssh | mulch_vm_backup | mulch_vm_config | mulch_vm_delete | mulch_vm_infos | mulch_vm_lock | mulch_vm_rebuild | mulch_vm_redefine | mulch_vm_start | mulch_vm_stop | mulch_vm_unlock | mulch_vm_activate | mulch_vm_deactivate | mulch_log | mulch_vm_console)
            __internal_list_vms
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
        mulch_key_right_list | mulch_key_right_add | mulch_key_right_remove)
            __internal_list_keys
            return
            ;;
        *)
            ;;
    esac
}
`
)
