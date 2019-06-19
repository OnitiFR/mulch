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
    if [ "$prev" =  "do" ]; then
        __internal_list_vms
    elif [ "${words[ $((c-2)) ]}" =  "do" ]; then
        __internal_list_actions $prev
    fi
}

__mulch_get_servers() {
    local out servers
    servers=$(egrep '^[[:blank:]]*name[[:blank:]]*=' ~/.mulch.toml | awk -F= '{print $2}')
    out=($(echo $servers))
    COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
}

__custom_func() {
    case ${last_command} in
        mulch_vm_create)
            __internal_list_toml_files
            return
            ;;
        mulch_ssh | mulch_vm_backup | mulch_vm_config | mulch_vm_delete | mulch_vm_infos | mulch_vm_lock | mulch_vm_rebuild | mulch_vm_redefine | mulch_vm_start | mulch_vm_stop | mulch_vm_unlock | mulch_vm_activate)
            __internal_list_vms
            return
            ;;
        mulch_backup_cat | mulch_backup_delete | mulch_backup_download | mulch_backup_mount)
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
        mulch_seed_status)
            __internal_list_seeds
            return
            ;;
        *)
            ;;
    esac
}
`
)
